package mcpauthz

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
)

const (
	MaxArgumentsBytes = 1 << 20
	MaxJSONDepth      = 32
	MaxJSONNodes      = 10000
)

var ErrInvalidArguments = errors.New("invalid MCP tool arguments")

// DecodeArguments parses the final request bytes exactly once. Number values
// stay json.Number until schema checks finish, so large integers cannot be
// rounded through float64 and then authorized with different bytes.
func DecodeArguments(raw []byte, schema json.RawMessage) (map[string]any, []byte, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if len(raw) > MaxArgumentsBytes {
		return nil, nil, fmt.Errorf("%w: size", ErrInvalidArguments)
	}
	if hasDuplicateJSONKeys(raw) != nil {
		return nil, nil, fmt.Errorf("%w: duplicate or trailing JSON", ErrInvalidArguments)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, nil, fmt.Errorf("%w: trailing JSON", ErrInvalidArguments)
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, nil, fmt.Errorf("%w: arguments must be an object", ErrInvalidArguments)
	}
	if err := ValidateSchema(schema); err != nil {
		return nil, nil, err
	}
	if err := validateJSONValue(object, schemaValue(schema), 0, newJSONBudget()); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: canonical encoding", ErrInvalidArguments)
	}
	return object, canonical, nil
}

// ValidateSchema accepts the deterministic JSON Schema subset used by MCP
// releases. References, dialect extensions and unknown keywords are rejected
// at publication/runtime preparation instead of being silently ignored.
func ValidateSchema(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: empty schema", ErrInvalidArguments)
	}
	if hasDuplicateJSONKeys(raw) != nil {
		return fmt.Errorf("%w: schema has duplicate or trailing JSON", ErrInvalidArguments)
	}
	var schema any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&schema); err != nil {
		return fmt.Errorf("%w: schema JSON: %v", ErrInvalidArguments, err)
	}
	if err := validateSchemaValue(schema, 0); err != nil {
		return fmt.Errorf("%w: schema: %v", ErrInvalidArguments, err)
	}
	return nil
}

var supportedSchemaKeywords = map[string]bool{
	"$schema": true, "type": true, "properties": true, "required": true,
	"additionalProperties": true, "items": true, "enum": true, "const": true,
	"description": true,
	"minimum":     true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
	"minLength": true, "maxLength": true, "pattern": true, "minItems": true,
	"maxItems": true, "minProperties": true, "maxProperties": true,
}

func schemaValue(raw json.RawMessage) any {
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&value)
	return value
}

func validateSchemaValue(value any, depth int) error {
	if depth > MaxJSONDepth {
		return errors.New("schema nesting exceeds limit")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("schema must be an object")
	}
	for key := range object {
		if !supportedSchemaKeywords[key] {
			return fmt.Errorf("unsupported keyword %q", key)
		}
	}
	if schema, ok := object["$schema"]; ok {
		name, ok := schema.(string)
		if !ok || name == "" || !strings.Contains(name, "json-schema") {
			return errors.New("unsupported schema dialect")
		}
	}
	if typeValue, ok := object["type"]; ok {
		if err := validateTypes(typeValue); err != nil {
			return err
		}
	}
	if properties, ok := object["properties"]; ok {
		items, ok := properties.(map[string]any)
		if !ok {
			return errors.New("properties must be an object")
		}
		for name, child := range items {
			if name == "" {
				return errors.New("empty property name")
			}
			if err := validateSchemaValue(child, depth+1); err != nil {
				return fmt.Errorf("property %q: %w", name, err)
			}
		}
	}
	if required, ok := object["required"]; ok {
		items, ok := required.([]any)
		if !ok {
			return errors.New("required must be an array")
		}
		seen := map[string]bool{}
		for _, item := range items {
			name, ok := item.(string)
			if !ok || name == "" || seen[name] {
				return errors.New("required must contain unique names")
			}
			seen[name] = true
		}
	}
	if additional, ok := object["additionalProperties"]; ok {
		switch v := additional.(type) {
		case bool:
		case map[string]any:
			if err := validateSchemaValue(v, depth+1); err != nil {
				return err
			}
		default:
			return errors.New("additionalProperties must be boolean or schema")
		}
	}
	if items, ok := object["items"]; ok {
		if err := validateSchemaValue(items, depth+1); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	if enum, ok := object["enum"]; ok {
		if values, ok := enum.([]any); !ok || len(values) == 0 {
			return errors.New("enum must be non-empty array")
		}
	}
	if pattern, ok := object["pattern"]; ok {
		value, ok := pattern.(string)
		if !ok {
			return errors.New("pattern must be string")
		}
		if _, err := regexp.Compile(value); err != nil {
			return fmt.Errorf("pattern: %w", err)
		}
	}
	for _, key := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"} {
		if v, ok := object[key]; ok {
			if _, ok := v.(json.Number); !ok {
				return fmt.Errorf("%s must be number", key)
			}
		}
	}
	for _, key := range []string{"minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties"} {
		if v, ok := object[key]; ok {
			n, ok := v.(json.Number)
			if !ok {
				return fmt.Errorf("%s must be integer", key)
			}
			if _, err := n.Int64(); err != nil {
				return fmt.Errorf("%s must be integer", key)
			}
		}
	}
	return nil
}

func validateTypes(value any) error {
	valid := map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}
	switch v := value.(type) {
	case string:
		if !valid[v] {
			return fmt.Errorf("unsupported type %q", v)
		}
	case []any:
		if len(v) == 0 {
			return errors.New("type array is empty")
		}
		seen := map[string]bool{}
		for _, item := range v {
			name, ok := item.(string)
			if !ok || !valid[name] || seen[name] {
				return errors.New("invalid type array")
			}
			seen[name] = true
		}
	default:
		return errors.New("type must be string or array")
	}
	return nil
}

type jsonBudget struct{ nodes int }

func newJSONBudget() *jsonBudget { return &jsonBudget{} }

func validateJSONValue(value any, schema any, depth int, budget *jsonBudget) error {
	if depth > MaxJSONDepth {
		return errors.New("JSON nesting exceeds limit")
	}
	budget.nodes++
	if budget.nodes > MaxJSONNodes {
		return errors.New("JSON node limit exceeded")
	}
	definition, ok := schema.(map[string]any)
	if !ok {
		return errors.New("schema must be object")
	}
	if enum, ok := definition["enum"].([]any); ok {
		matched := false
		for _, candidate := range enum {
			if jsonEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("value is not in enum")
		}
	}
	if candidate, ok := definition["const"]; ok && !jsonEqual(value, candidate) {
		return errors.New("value does not match const")
	}
	types, ok := definition["type"]
	if ok && !matchesAnyType(value, types) {
		return errors.New("value has wrong type")
	}
	if object, ok := value.(map[string]any); ok {
		if n, ok := integerConstraint(definition, "minProperties"); ok && len(object) < n {
			return errors.New("too few properties")
		}
		if n, ok := integerConstraint(definition, "maxProperties"); ok && len(object) > n {
			return errors.New("too many properties")
		}
		if required, ok := definition["required"].([]any); ok {
			for _, item := range required {
				name := item.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("missing required property %q", name)
				}
			}
		}
		properties, _ := definition["properties"].(map[string]any)
		additional, hasAdditional := definition["additionalProperties"]
		for name, item := range object {
			child, known := properties[name]
			if !known && hasAdditional {
				switch extra := additional.(type) {
				case bool:
					if !extra {
						return fmt.Errorf("additional property %q is not allowed", name)
					}
				case map[string]any:
					child = extra
					known = true
				}
			}
			if known {
				if err := validateJSONValue(item, child, depth+1, budget); err != nil {
					return fmt.Errorf("property %q: %w", name, err)
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		if n, ok := integerConstraint(definition, "minItems"); ok && len(array) < n {
			return errors.New("too few items")
		}
		if n, ok := integerConstraint(definition, "maxItems"); ok && len(array) > n {
			return errors.New("too many items")
		}
		if itemSchema, ok := definition["items"]; ok {
			for index, item := range array {
				if err := validateJSONValue(item, itemSchema, depth+1, budget); err != nil {
					return fmt.Errorf("item %d: %w", index, err)
				}
			}
		}
	}
	if text, ok := value.(string); ok {
		if n, ok := integerConstraint(definition, "minLength"); ok && len([]rune(text)) < n {
			return errors.New("string is too short")
		}
		if n, ok := integerConstraint(definition, "maxLength"); ok && len([]rune(text)) > n {
			return errors.New("string is too long")
		}
		if pattern, ok := definition["pattern"].(string); ok {
			matched, _ := regexp.MatchString(pattern, text)
			if !matched {
				return errors.New("string does not match pattern")
			}
		}
	}
	if number, ok := jsonNumber(value); ok {
		if err := validateNumber(number, definition); err != nil {
			return err
		}
	}
	return nil
}

func integerConstraint(definition map[string]any, key string) (int, bool) {
	n, ok := definition[key].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := n.Int64()
	return int(value), err == nil
}
func jsonNumber(value any) (json.Number, bool) { n, ok := value.(json.Number); return n, ok }
func validateNumber(value json.Number, definition map[string]any) error {
	actual, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return errors.New("invalid number")
	}
	for _, rule := range []struct {
		key    string
		lower  bool
		strict bool
	}{{"minimum", true, false}, {"exclusiveMinimum", true, true}, {"maximum", false, false}, {"exclusiveMaximum", false, true}} {
		if raw, exists := definition[rule.key].(json.Number); exists {
			bound, parsed := new(big.Rat).SetString(raw.String())
			if !parsed {
				return errors.New("invalid numeric constraint")
			}
			comparison := actual.Cmp(bound)
			if rule.lower && ((rule.strict && comparison <= 0) || (!rule.strict && comparison < 0)) {
				return fmt.Errorf("number below %s", rule.key)
			}
			if !rule.lower && ((rule.strict && comparison >= 0) || (!rule.strict && comparison > 0)) {
				return fmt.Errorf("number above %s", rule.key)
			}
		}
	}
	return nil
}
func matchesAnyType(value, typeValue any) bool {
	types := []string{}
	switch v := typeValue.(type) {
	case string:
		types = []string{v}
	case []any:
		for _, item := range v {
			if name, ok := item.(string); ok {
				types = append(types, name)
			}
		}
	}
	for _, name := range types {
		switch name {
		case "object":
			if _, ok := value.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := value.([]any); ok {
				return true
			}
		case "string":
			if _, ok := value.(string); ok {
				return true
			}
		case "number":
			if _, ok := jsonNumber(value); ok {
				return true
			}
		case "integer":
			if n, ok := jsonNumber(value); ok {
				if rational, parsed := new(big.Rat).SetString(n.String()); parsed && rational.IsInt() {
					return true
				}
			}
		case "boolean":
			if _, ok := value.(bool); ok {
				return true
			}
		case "null":
			if value == nil {
				return true
			}
		}
	}
	return false
}
func jsonEqual(left, right any) bool {
	lb, _ := json.Marshal(left)
	rb, _ := json.Marshal(right)
	return bytes.Equal(lb, rb)
}

// Stable schema digest helper used by release/tool binding checks.
func SchemaDigest(raw json.RawMessage) (string, error) {
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return "", err
	}
	if err := ValidateSchema(raw); err != nil {
		return "", err
	}
	return CanonicalDigest(value)
}
func SortedSchemaKeys(value map[string]any) []string {
	out := make([]string, 0, len(value))
	for key := range value {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
