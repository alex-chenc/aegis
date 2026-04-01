package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// MarkdownParser Markdown 文件解析器
type MarkdownParser struct{}

func (p *MarkdownParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read markdown file: %w", err)
	}

	return strings.TrimSpace(string(content)), nil
}

func (p *MarkdownParser) SupportedTypes() []string {
	return []string{".md", ".markdown"}
}
