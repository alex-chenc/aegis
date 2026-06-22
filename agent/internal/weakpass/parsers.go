package weakpass

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var errFieldNotFound = errors.New("field_not_found")

func defaultExtractors(application string) []CredentialExtractor {
	switch strings.ToLower(application) {
	case "linux_shadow", "linux local account", "linux":
		return []CredentialExtractor{{Type: "shadow", SourceKind: "system_account", FormatHint: "salted_hash"}}
	case "redis":
		return []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: "plaintext"}}
	default:
		return []CredentialExtractor{{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "password", FormatHint: "plaintext"}}
	}
}

func parseCredentialFile(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	parserType := strings.ToLower(strings.TrimSpace(extractor.Type))
	if parserType == "" {
		parserType = "line_key_value"
	}
	extractor.Type = parserType
	switch parserType {
	case "shadow":
		return parseShadow(app, path, content, extractor)
	case "ini":
		return parseINI(app, path, content, extractor)
	case "yaml", "yml":
		return parseYAML(app, path, content, extractor)
	case "json":
		return parseJSON(app, path, content, extractor)
	case "properties":
		return parseProperties(app, path, content, extractor)
	case "line_key_value":
		return parseLineKeyValue(app, path, content, extractor)
	case "htpasswd":
		return parseHTPasswd(app, path, content, extractor)
	default:
		return nil, fmt.Errorf("%s: parser %q", ErrUnsupportedFormat, parserType)
	}
}

func parseShadow(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	var records []CredentialRecord
	for _, line := range splitLines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		account := parts[0]
		value := parts[1]
		if value == "" {
			rec := newRecord(app, path, extractor, account, "empty_password", "shadow.password")
			rec.CredentialType = CredentialTypePlaintext
			rec.AlgorithmHint = "empty"
			records = append(records, rec)
			continue
		}
		if strings.HasPrefix(value, "!") || strings.HasPrefix(value, "*") {
			continue
		}
		rec := newRecord(app, path, extractor, account, value, "shadow.password")
		rec.CredentialType, rec.Salt, rec.AlgorithmHint = classifyShadow(value)
		rec.Confidence = 1.0
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, errFieldNotFound
	}
	return records, nil
}

func parseINI(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	currentSection := ""
	values := map[string]map[string]string{"": {}}
	for _, line := range splitLines(content) {
		line = strings.TrimSpace(stripInlineComment(line))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			currentSection = strings.TrimSpace(line[1:strings.Index(line, "]")])
			if _, ok := values[currentSection]; !ok {
				values[currentSection] = map[string]string{}
			}
			continue
		}
		key, val, ok := splitKV(line)
		if ok {
			values[currentSection][key] = val
		}
	}
	section := extractor.Section
	if _, ok := values[section]; !ok {
		return nil, errFieldNotFound
	}
	account := values[section][extractor.AccountSelector]
	password := values[section][extractor.PasswordSelector]
	if password == "" {
		return nil, errFieldNotFound
	}
	return []CredentialRecord{newRecord(app, path, extractor, account, password, joinField(section, extractor.PasswordSelector))}, nil
}

func parseProperties(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	values := map[string]string{}
	for _, line := range splitLines(content) {
		line = strings.TrimSpace(stripInlineComment(line))
		if line == "" {
			continue
		}
		key, val, ok := splitKV(line)
		if ok {
			values[key] = val
		}
	}
	password := values[extractor.PasswordSelector]
	if password == "" {
		return nil, errFieldNotFound
	}
	return []CredentialRecord{newRecord(app, path, extractor, values[extractor.AccountSelector], password, extractor.PasswordSelector)}, nil
}

func parseLineKeyValue(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	var records []CredentialRecord
	account := ""
	for _, line := range splitLines(content) {
		line = strings.TrimSpace(stripInlineComment(line))
		if line == "" {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		if extractor.AccountSelector != "" && strings.EqualFold(key, extractor.AccountSelector) {
			account = val
		}
		if extractor.PasswordSelector != "" && strings.EqualFold(key, extractor.PasswordSelector) && val != "" {
			records = append(records, newRecord(app, path, extractor, account, val, key))
		}
	}
	if len(records) == 0 {
		return nil, errFieldNotFound
	}
	return records, nil
}

func parseHTPasswd(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	var records []CredentialRecord
	for _, line := range splitLines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		rec := newRecord(app, path, extractor, parts[0], parts[1], "htpasswd.password")
		rec.CredentialType, rec.Salt, rec.AlgorithmHint = classifyCredential(parts[1], "hash")
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, errFieldNotFound
	}
	return records, nil
}

func parseJSON(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	var data interface{}
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	account := lookupSelector(data, extractor.AccountSelector)
	password := lookupSelector(data, extractor.PasswordSelector)
	if password == "" {
		return nil, errFieldNotFound
	}
	return []CredentialRecord{newRecord(app, path, extractor, account, password, extractor.PasswordSelector)}, nil
}

func parseYAML(app ApplicationCollectPlan, path string, content []byte, extractor CredentialExtractor) ([]CredentialRecord, error) {
	var data interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	data = normalizeYAML(data)
	account := lookupSelector(data, extractor.AccountSelector)
	password := lookupSelector(data, extractor.PasswordSelector)
	if password == "" {
		return nil, errFieldNotFound
	}
	return []CredentialRecord{newRecord(app, path, extractor, account, password, extractor.PasswordSelector)}, nil
}

func splitLines(content []byte) []string {
	return strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
}

func stripInlineComment(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return ""
	}
	return line
}

func splitKV(line string) (string, string, bool) {
	if idx := strings.Index(line, "="); idx >= 0 {
		return strings.TrimSpace(line[:idx]), unquote(strings.TrimSpace(line[idx+1:])), true
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return strings.TrimSpace(fields[0]), unquote(strings.Join(fields[1:], " ")), true
	}
	return "", "", false
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}

func joinField(section, key string) string {
	if section == "" {
		return key
	}
	return section + "." + key
}

func normalizeYAML(in interface{}) interface{} {
	switch v := in.(type) {
	case map[interface{}]interface{}:
		out := map[string]interface{}{}
		for key, value := range v {
			out[fmt.Sprint(key)] = normalizeYAML(value)
		}
		return out
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, value := range v {
			out[key] = normalizeYAML(value)
		}
		return out
	case []interface{}:
		for i := range v {
			v[i] = normalizeYAML(v[i])
		}
		return v
	default:
		return in
	}
}

var selectorPartRE = regexp.MustCompile(`^([^\[]+)(?:\[(\d+)\])?$`)

func lookupSelector(data interface{}, selector string) string {
	if selector == "" {
		return ""
	}
	current := data
	for _, part := range strings.Split(selector, ".") {
		matches := selectorPartRE.FindStringSubmatch(part)
		if len(matches) == 0 {
			return ""
		}
		key := matches[1]
		obj, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = obj[key]
		if !ok {
			return ""
		}
		if matches[2] != "" {
			var idx int
			if _, err := fmt.Sscanf(matches[2], "%d", &idx); err != nil {
				return ""
			}
			arr, ok := current.([]interface{})
			if !ok || idx < 0 || idx >= len(arr) {
				return ""
			}
			current = arr[idx]
		}
	}
	switch v := current.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func classifyCredential(value, hint string) (string, string, string) {
	hint = strings.ToLower(strings.TrimSpace(hint))
	if hint == CredentialTypePlaintext {
		return CredentialTypePlaintext, "", ""
	}
	if hint == CredentialTypeEncryptedBlob || hint == CredentialTypeAuthString || hint == CredentialTypeUnknown {
		return hint, "", ""
	}
	if credType, salt, algorithm := classifyShadow(value); algorithm != "" {
		return credType, salt, algorithm
	}
	if strings.HasPrefix(value, "$apr1$") {
		parts := strings.Split(value, "$")
		if len(parts) > 2 {
			return CredentialTypeSaltedHash, parts[2], "apr1"
		}
		return CredentialTypeHash, "", "apr1"
	}
	if strings.HasPrefix(value, "{SHA}") {
		return CredentialTypeHash, "", "sha1"
	}
	if hint == CredentialTypeHash || hint == CredentialTypeSaltedHash {
		return hint, "", ""
	}
	return CredentialTypePlaintext, "", ""
}

func classifyShadow(value string) (string, string, string) {
	prefixToAlgorithm := map[string]string{
		"$1$":  "md5-crypt",
		"$2a$": "bcrypt",
		"$2b$": "bcrypt",
		"$2y$": "bcrypt",
		"$5$":  "sha256-crypt",
		"$6$":  "sha512-crypt",
		"$y$":  "yescrypt",
	}
	for prefix, algorithm := range prefixToAlgorithm {
		if strings.HasPrefix(value, prefix) {
			parts := strings.Split(value, "$")
			salt := ""
			if len(parts) > 2 {
				salt = parts[2]
			}
			return CredentialTypeSaltedHash, salt, algorithm
		}
	}
	return CredentialTypeHash, "", ""
}
