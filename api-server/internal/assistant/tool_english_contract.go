package assistant

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateToolEnglishContract rejects non-English model-facing tool metadata.
// Aliases are intentionally excluded because they are multilingual retrieval
// hints and are never emitted as the model's executable contract.
func ValidateToolEnglishContract(tool *ToolSpec) error {
	if tool == nil {
		return fmt.Errorf("tool is nil")
	}
	for field, value := range map[string]string{
		"description":       tool.Description,
		"model_description": tool.ModelDescription,
		"service_notes":     tool.ServiceBinding.Notes,
	} {
		if containsHan(value) {
			return fmt.Errorf("tool %s %s must be English", tool.Name, field)
		}
	}
	for index, tag := range tool.Tags {
		if containsHan(tag) {
			return fmt.Errorf("tool %s tags[%d] must be English", tool.Name, index)
		}
	}
	if err := validateEnglishSchemaText(tool.ArgsSchema, "args_schema"); err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}
	if err := validateEnglishSchemaText(tool.ResultSchema, "result_schema"); err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}
	return nil
}

func validateEnglishSchemaText(value interface{}, path string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := typed[key]
			childPath := path + "." + key
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "description", "title", "$comment":
				if text, ok := item.(string); ok && containsHan(text) {
					return fmt.Errorf("%s must be English", childPath)
				}
			}
			if err := validateEnglishSchemaText(item, childPath); err != nil {
				return err
			}
		}
	case []interface{}:
		for index, item := range typed {
			if err := validateEnglishSchemaText(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateModelFacingEnglish checks every enabled registered tool before the
// assistant begins serving requests.
func (r *ToolRegistry) ValidateModelFacingEnglish() error {
	if r == nil {
		return fmt.Errorf("tool registry is nil")
	}
	tools := r.List()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	for _, tool := range tools {
		if tool == nil || !tool.Enabled {
			continue
		}
		if err := ValidateToolEnglishContract(tool); err != nil {
			return err
		}
	}
	return nil
}
