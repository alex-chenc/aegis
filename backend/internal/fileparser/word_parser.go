package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// WordParser Word 文档解析器（简化版本）
type WordParser struct{}

func (p *WordParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read Word file: %w", err)
	}

	// 简化的 Word 解析 - 实际项目中应使用 go-fitz 或其他 DOCX 库
	return strings.TrimSpace(string(content)), nil
}

func (p *WordParser) SupportedTypes() []string {
	return []string{".docx", ".doc"}
}
