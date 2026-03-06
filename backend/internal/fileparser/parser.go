package fileparser

import (
	"fmt"
	"io"
	"strings"
)

// FileParser 文件解析器接口
type FileParser interface {
	// Parse 解析文件内容，返回提取出的纯文本
	Parse(reader io.Reader) (string, error)
	// SupportedTypes 返回该解析器支持的文件扩展名列表
	SupportedTypes() []string
}

// FileType 文件类型枚举
type FileType string

const (
	FileTypePDF      FileType = "pdf"
	FileTypeWord     FileType = "docx"
	FileTypeYAML     FileType = "yaml"
	FileTypeExcel    FileType = "xlsx"
	FileTypeTXT      FileType = "txt"
	FileTypeMarkdown FileType = "md"
)

// GetParser 根据文件类型返回对应的解析器
func GetParser(fileType FileType) (FileParser, error) {
	switch fileType {
	case FileTypeYAML:
		return &YAMLParser{}, nil
	case FileTypeTXT:
		return &TextParser{}, nil
	case FileTypeMarkdown:
		return &MarkdownParser{}, nil
	case FileTypePDF:
		return &PDFParser{}, nil
	case FileTypeWord:
		return &WordParser{}, nil
	case FileTypeExcel:
		return &ExcelParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}
}

// GetParserByExtension 根据文件扩展名返回解析器
func GetParserByExtension(filename string) (FileParser, error) {
	ext := strings.ToLower(filename[strings.LastIndex(filename, ".")+1:])

	switch ext {
	case "yaml", "yml":
		return &YAMLParser{}, nil
	case "txt", "text":
		return &TextParser{}, nil
	case "md", "markdown":
		return &MarkdownParser{}, nil
	case "pdf":
		return &PDFParser{}, nil
	case "docx", "doc":
		return &WordParser{}, nil
	case "xlsx", "xls":
		return &ExcelParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// SupportedExtensions 返回所有支持的扩展名
func SupportedExtensions() []string {
	return []string{".pdf", ".docx", ".doc", ".yaml", ".yml", ".xlsx", ".xls", ".txt", ".md", ".markdown"}
}
