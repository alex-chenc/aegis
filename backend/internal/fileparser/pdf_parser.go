package fileparser

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

type PDFParser struct{}

func (p *PDFParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read PDF file: %w", err)
	}

	pdfReader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("failed to parse PDF: %w (请确保上传的是有效的PDF文件)", err)
	}

	var text strings.Builder
	totalPage := pdfReader.NumPage()

	for i := 1; i <= totalPage; i++ {
		page := pdfReader.Page(i)
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text.WriteString(pageText)
		text.WriteString("\n")
	}

	result := strings.TrimSpace(text.String())
	if result == "" {
		return "", fmt.Errorf("PDF 文件不包含可提取的文本内容，可能是扫描件或图片PDF")
	}

	return result, nil
}

func (p *PDFParser) SupportedTypes() []string {
	return []string{".pdf"}
}
