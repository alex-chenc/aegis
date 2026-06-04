package assets

import (
	"regexp"
	"strings"
)

// sensitivePatterns 敏感参数模式列表
var sensitivePatterns = []*regexp.Regexp{
	// password 相关
	regexp.MustCompile(`(?i)(--password[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(--passwd[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(-p[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(password[=:])([^&\s]+)`),
	// token 相关
	regexp.MustCompile(`(?i)(--token[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(--api-token[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(token[=:])([^&\s]+)`),
	// secret 相关
	regexp.MustCompile(`(?i)(--secret[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(--secret-key[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(secret[=:])([^&\s]+)`),
	// access key 相关
	regexp.MustCompile(`(?i)(--access-key[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(--access-key-id[=\s]+)\S+`),
	regexp.MustCompile(`(?i)(access.key[=:])([^&\s]+)`),
	// AWS 相关
	regexp.MustCompile(`(?i)(AWS_ACCESS_KEY_ID[=])([^&\s]+)`),
	regexp.MustCompile(`(?i)(AWS_SECRET_ACCESS_KEY[=])([^&\s]+)`),
	// URL 中的用户名密码 (使用贪婪匹配到最后一个 @ 前)
	regexp.MustCompile(`(://[^:]+:).+(@[^@]*$)`),
}

// RedactCmdline 对命令行进行脱敏处理
// 保留参数名，将敏感值替换为 ***
func RedactCmdline(cmdline string) string {
	if cmdline == "" {
		return cmdline
	}

	result := cmdline
	for _, pattern := range sensitivePatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			// URL 中的密码特殊处理
			if strings.Contains(match, "://") {
				return pattern.ReplaceAllString(match, "${1}***${2}")
			}
			// 其他参数：保留参数名和分隔符，替换值
			return pattern.ReplaceAllString(match, "${1}***")
		})
	}

	return result
}

// configPatterns 配置文件中的敏感字段模式
var configPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password\s*[=:]\s*)[^\s;}\n]+`),
	regexp.MustCompile(`(?i)(token\s*[=:]\s*)[^\s;}\n]+`),
	regexp.MustCompile(`(?i)(secret\s*[=:]\s*)[^\s;}\n]+`),
	regexp.MustCompile(`(?i)(api_key\s*[=:]\s*)[^\s;}\n]+`),
	regexp.MustCompile(`(?i)(access_key\s*[=:]\s*)[^\s;}\n]+`),
	regexp.MustCompile(`(?i)(private_key\s*[=:]\s*)[^\s;}\n]+`),
}

// RedactConfigSummary 对配置内容进行脱敏
func RedactConfigSummary(content string) string {
	if content == "" {
		return content
	}

	result := content
	for _, pattern := range configPatterns {
		result = pattern.ReplaceAllString(result, "${1}***")
	}

	return result
}
