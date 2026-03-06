package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// PDFParser PDF 文件解析器（简化版本）
type PDFParser struct{}

func (p *PDFParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read PDF file: %w", err)
	}

	// 简化的 PDF 解析 - 实际项目中应使用 pdfcpu 或其他 PDF 库
	// 这里返回原始字节，后续需要集成真正的 PDF 解析库
	return strings.TrimSpace(string(content)), nil
}

func (p *PDFParser) SupportedTypes() []string {
	return []string{".pdf"}
}
