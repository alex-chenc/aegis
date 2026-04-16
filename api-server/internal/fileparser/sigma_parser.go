package fileparser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// SigmaRuleParser Sigma规则解析器
type SigmaRuleParser struct{}

// SigmaRule Sigma规则结构（解析用）
type SigmaRule struct {
	Title       string                 `yaml:"title"`
	ID         string                 `yaml:"id"`
	Status     string                 `yaml:"status"`
	Description string                 `yaml:"description"`
	Tags       []string               `yaml:"tags"`
	Level      string                 `yaml:"level"`
	Logsource  map[string]interface{} `yaml:"logsource"`
	Detection  map[string]interface{} `yaml:"detection"`
	Fields     []string               `yaml:"fields"`
	FalsePositives []string           `yaml:"falsepositives"`
}

// Parse 从YAML内容解析Sigma规则
func (p *SigmaRuleParser) Parse(content []byte) (*SigmaRule, error) {
	var rule SigmaRule
	if err := yaml.Unmarshal(content, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 验证必填字段
	if rule.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if len(rule.Detection) == 0 {
		return nil, fmt.Errorf("detection is required")
	}

	// 生成规则ID（如果未提供）
	if rule.ID == "" {
		rule.ID = generateRuleID(rule.Title)
	}

	// 设置默认状态
	if rule.Status == "" {
		rule.Status = "experimental"
	}

	return &rule, nil
}

// ParseMITRETags 从tags和title解析MITRE ID
func ParseMITRETags(tags []string, title string) []string {
	mitreTags := []string{}
	mitreRegex := regexp.MustCompile(`T\d{4}(?:\.\d{3})?`)

	// 从tags中提取
	for _, tag := range tags {
		matches := mitreRegex.FindAllString(tag, -1)
		mitreTags = append(mitreTags, matches...)
	}

	// 从title中提取
	titleMatches := mitreRegex.FindAllString(title, -1)
	mitreTags = append(mitreTags, titleMatches...)

	// 去重
	seen := make(map[string]bool)
	result := []string{}
	for _, t := range mitreTags {
		upper := strings.ToUpper(t)
		if !seen[upper] {
			seen[upper] = true
			result = append(result, upper)
		}
	}

	return result
}

// generateRuleID 从title生成规则ID
func generateRuleID(title string) string {
	// 转小写，替换空格和特殊字符
	id := strings.ToLower(title)
	id = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(id, "_")
	id = strings.Trim(id, "_")
	// 截断长度
	if len(id) > 50 {
		id = id[:50]
	}
	// 添加随机后缀
	return fmt.Sprintf("%s_%s", id, randomString(6))
}

// randomString 生成随机字符串
func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// ComputeFileHash 计算文件内容的SHA256哈希
func ComputeFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// NormalizeMitreID 标准化MITRE ID格式
func NormalizeMitreID(mitreID string) string {
	upper := strings.ToUpper(mitreID)
	if !strings.HasPrefix(upper, "T") {
		upper = "T" + upper
	}
	return upper
}

// ExtractMITREFromTag 从tag字符串中提取MITRE ID
// 例如: "attack.t1059.004" -> "T1059.004"
func ExtractMITREFromTag(tag string) string {
	mitreRegex := regexp.MustCompile(`T\d{4}(?:\.\d{3})?`)
	matches := mitreRegex.FindString(strings.ToUpper(tag))
	return matches
}

// IsValidSigmaRule 验证是否为有效的Sigma规则
func IsValidSigmaRule(rule *SigmaRule) bool {
	return rule.Title != "" && len(rule.Detection) > 0
}

// init 初始化随机数生成器
func init() {
	rand.Seed(time.Now().UnixNano())
}

// ContainsValidMITRE 检查tags是否包含有效的MITRE ID
func ContainsValidMITRE(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(strings.ToLower(tag), "attack.t") {
			return true
		}
	}
	return false
}

// CleanSigmaRule 清理Sigma规则中的控制字符
func CleanSigmaRule(content string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, content)
}
