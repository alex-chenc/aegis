package sigma

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	Title       string    `yaml:"title"`
	ID          string    `yaml:"id"`
	Status      string    `yaml:"status"`
	Description string    `yaml:"description"`
	Logsource   Logsource `yaml:"logsource"`
	Detection   Detection `yaml:"detection"`
	Level       string    `yaml:"level"`
	Tags        []string  `yaml:"tags"`
}

type Logsource struct {
	Category string `yaml:"category"`
	Product  string `yaml:"product"`
}

type Detection struct {
	Selections map[string]interface{} `yaml:",inline"`
	Condition  string                 `yaml:"condition"`
}

func ParseRule(content []byte) (*Rule, error) {
	var rule Rule
	if err := yaml.Unmarshal(content, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse sigma rule: %w", err)
	}
	if rule.ID == "" {
		return nil, fmt.Errorf("sigma rule missing id field")
	}
	return &rule, nil
}

// ParseRules parses multiple Sigma rules from YAML content (separated by ---)
func ParseRules(content []byte) ([]Rule, error) {
	var rules []Rule
	decoder := yaml.NewDecoder(bytes.NewReader(content))

	for {
		var rule Rule
		if err := decoder.Decode(&rule); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("failed to parse sigma rule: %w", err)
		}
		if rule.ID != "" {
			rules = append(rules, rule)
		}
	}

	return rules, nil
}
