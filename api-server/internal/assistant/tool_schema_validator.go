package assistant

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ValidateToolArgs applies the model-facing JSON Schema again at the trusted
// backend boundary. It covers the schema features used by built-in Aegis tools
// and prevents callers from bypassing agent-runtime validation.
func ValidateToolArgs(schema map[string]interface{}, args map[string]interface{}) error {
	if len(schema) == 0 {
		return nil
	}
	return validateSchemaValue(schema, args, "$")
}

func validateSchemaValue(schema map[string]interface{}, value interface{}, path string) error {
	if value == nil {
		if schemaAllowsNull(schema) {
			return nil
		}
		return fmt.Errorf("%s is null", path)
	}
	if enumValues := schemaEnumValues(schema["enum"]); len(enumValues) > 0 && !schemaEnumContains(enumValues, value) {
		return fmt.Errorf("%s must be one of %v", path, enumValues)
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		if err := validateRequiredProperties(schema, object, path); err != nil {
			return err
		}
		properties, _ := schema["properties"].(map[string]interface{})
		for key, item := range object {
			propertySchema, declared := properties[key]
			if !declared {
				if additional, exists := schema["additionalProperties"].(bool); exists && !additional {
					return fmt.Errorf("%s.%s is not allowed", path, key)
				}
				continue
			}
			typedSchema, ok := propertySchema.(map[string]interface{})
			if !ok {
				continue
			}
			if err := validateSchemaValue(typedSchema, item, path+"."+key); err != nil {
				return err
			}
		}
	case "array":
		sliceValue := reflect.ValueOf(value)
		if sliceValue.Kind() != reflect.Array && sliceValue.Kind() != reflect.Slice {
			return fmt.Errorf("%s must be an array", path)
		}
		if minimum, ok := schemaNumber(schema["minItems"]); ok && float64(sliceValue.Len()) < minimum {
			return fmt.Errorf("%s must contain at least %d items", path, int(minimum))
		}
		if maximum, ok := schemaNumber(schema["maxItems"]); ok && float64(sliceValue.Len()) > maximum {
			return fmt.Errorf("%s must contain at most %d items", path, int(maximum))
		}
		itemSchema, _ := schema["items"].(map[string]interface{})
		for index := 0; index < sliceValue.Len() && itemSchema != nil; index++ {
			if err := validateSchemaValue(itemSchema, sliceValue.Index(index).Interface(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if minimum, ok := schemaNumber(schema["minLength"]); ok && float64(len([]rune(text))) < minimum {
			return fmt.Errorf("%s is shorter than minLength %d", path, int(minimum))
		}
		if maximum, ok := schemaNumber(schema["maxLength"]); ok && float64(len([]rune(text))) > maximum {
			return fmt.Errorf("%s is longer than maxLength %d", path, int(maximum))
		}
		if pattern, ok := schema["pattern"].(string); ok && pattern != "" {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("%s schema pattern is invalid: %w", path, err)
			}
			if !compiled.MatchString(text) {
				return fmt.Errorf("%s does not match the required pattern", path)
			}
		}
		if err := validateStringFormat(path, text, fmt.Sprint(schema["format"])); err != nil {
			return err
		}
	case "integer":
		number, ok := schemaNumber(value)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("%s must be an integer", path)
		}
		if err := validateNumberRange(schema, number, path); err != nil {
			return err
		}
	case "number":
		number, ok := schemaNumber(value)
		if !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		if err := validateNumberRange(schema, number, path); err != nil {
			return err
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	}
	return nil
}

func schemaEnumValues(value interface{}) []interface{} {
	switch values := value.(type) {
	case []interface{}:
		return values
	case []string:
		result := make([]interface{}, 0, len(values))
		for _, item := range values {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func validateRequiredProperties(schema map[string]interface{}, object map[string]interface{}, path string) error {
	var required []string
	switch values := schema["required"].(type) {
	case []string:
		required = values
	case []interface{}:
		for _, value := range values {
			if name, ok := value.(string); ok {
				required = append(required, name)
			}
		}
	}
	for _, name := range required {
		if _, exists := object[name]; !exists {
			return fmt.Errorf("%s.%s is required", path, name)
		}
	}
	return nil
}

func validateNumberRange(schema map[string]interface{}, value float64, path string) error {
	if minimum, ok := schemaNumber(schema["minimum"]); ok && value < minimum {
		return fmt.Errorf("%s must be at least %v", path, minimum)
	}
	if maximum, ok := schemaNumber(schema["maximum"]); ok && value > maximum {
		return fmt.Errorf("%s must be at most %v", path, maximum)
	}
	return nil
}

func validateStringFormat(path, value, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "<nil>":
		return nil
	case "uuid":
		if _, err := uuid.Parse(value); err != nil {
			return fmt.Errorf("%s must be a UUID", path)
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("%s must be an RFC3339 date-time", path)
		}
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return fmt.Errorf("%s must be an ISO date", path)
		}
	}
	return nil
}

func schemaAllowsNull(schema map[string]interface{}) bool {
	values, ok := schema["type"].([]interface{})
	if !ok {
		return false
	}
	for _, value := range values {
		if value == "null" {
			return true
		}
	}
	return false
}

func schemaEnumContains(values []interface{}, wanted interface{}) bool {
	wantedJSON, _ := json.Marshal(wanted)
	for _, value := range values {
		valueJSON, _ := json.Marshal(value)
		if string(valueJSON) == string(wantedJSON) {
			return true
		}
	}
	return false
}

func schemaNumber(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case float32:
		return float64(number), true
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
