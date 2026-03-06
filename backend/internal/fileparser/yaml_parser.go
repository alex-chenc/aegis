package fileparser

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLParser YAML 文件解析器
type YAMLParser struct{}

func (p *YAMLParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read YAML file: %w", err)
	}

	// 验证 YAML 格式
	var data interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return "", fmt.Errorf("invalid YAML format: %w", err)
	}

	// 返回原始文本内容
	return strings.TrimSpace(string(content)), nil
}

func (p *YAMLParser) SupportedTypes() []string {
	return []string{".yaml", ".yml"}
}
