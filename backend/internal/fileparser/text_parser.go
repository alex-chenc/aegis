package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// TextParser 纯文本文件解析器
type TextParser struct{}

func (p *TextParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read text file: %w", err)
	}

	return strings.TrimSpace(string(content)), nil
}

func (p *TextParser) SupportedTypes() []string {
	return []string{".txt", ".text"}
}
