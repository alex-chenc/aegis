package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// ExcelParser Excel 文件解析器（简化版本）
type ExcelParser struct{}

func (p *ExcelParser) Parse(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read Excel file: %w", err)
	}

	// 简化的 Excel 解析 - 实际项目中应使用 excelize 或其他 XLSX 库
	return strings.TrimSpace(string(content)), nil
}

func (p *ExcelParser) SupportedTypes() []string {
	return []string{".xlsx", ".xls"}
}
