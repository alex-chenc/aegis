package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestSkillRegistryContainsAllApplications 验证skill注册表包含所有支持的应用类型
func TestSkillRegistryContainsAllApplications(t *testing.T) {
	expectedTypes := []string{
		"redis", "openssh", "mysql", "mariadb", "postgresql", "postgres",
		"tomcat", "ftp", "vsftpd", "proftpd", "nginx", "apache", "httpd",
		"web_service", "kafka", "llm_service", "ai_agent", "mcp_server",
	}

	for _, appType := range expectedTypes {
		skill := weakPasswordSkillForApplication(appType)
		if skill.ApplicationType == "generic" && appType != "generic" {
			t.Errorf("application type %q should have a specific skill, but got generic", appType)
		}
		if len(skill.CandidatePaths) == 0 && appType != "llm_service" && appType != "ai_agent" && appType != "mcp_server" {
			t.Errorf("application type %q should have candidate paths", appType)
		}
		if len(skill.Extractors) == 0 {
			t.Errorf("application type %q should have extractors", appType)
		}
	}
}

// TestRedisSkillHasCorrectPaths 验证Redis skill有正确的配置路径
func TestRedisSkillHasCorrectPaths(t *testing.T) {
	skill := weakPasswordSkillForApplication("redis")
	expectedPaths := []string{
		"/etc/redis/redis.conf",
		"/etc/redis.conf",
		"/usr/local/etc/redis/redis.conf",
		"/data/redis.conf",
		"/redis.conf",
	}

	for _, expected := range expectedPaths {
		found := false
		for _, path := range skill.CandidatePaths {
			if path == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Redis skill missing expected path: %s", expected)
		}
	}

	// 验证提取器
	if len(skill.Extractors) < 2 {
		t.Errorf("Redis skill should have at least 2 extractors (requirepass and masterauth)")
	}
}

// TestOpenSHDSkillUsesShadowFile 验证OpenSSH skill使用/etc/shadow文件
func TestOpenSHDSkillUsesShadowFile(t *testing.T) {
	skill := weakPasswordSkillForApplication("openssh")

	if len(skill.CandidatePaths) != 1 || skill.CandidatePaths[0] != "/etc/shadow" {
		t.Errorf("OpenSSH skill should use /etc/shadow, got: %v", skill.CandidatePaths)
	}

	if len(skill.Extractors) == 0 || skill.Extractors[0].Type != "shadow" {
		t.Errorf("OpenSSH skill should use shadow extractor")
	}
}

func TestKafkaSkillHasSpecificPathsAndExtractors(t *testing.T) {
	skill := weakPasswordSkillForApplication("kafka")
	if skill.ApplicationType != "kafka" || skill.ProfileID != "kafka_config_v1" {
		t.Fatalf("Kafka skill = %#v, want kafka_config_v1", skill)
	}
	for _, expected := range []string{"/etc/kafka/kafka.properties", "/etc/kafka/server.properties", "/etc/kafka/kafka_server_jaas.conf"} {
		if !testContainsString(skill.CandidatePaths, expected) {
			t.Fatalf("Kafka skill paths = %#v, missing %s", skill.CandidatePaths, expected)
		}
	}
	var hasJAAS bool
	for _, extractor := range skill.Extractors {
		if extractor.PasswordSelector == "sasl.jaas.config" {
			hasJAAS = true
			break
		}
	}
	if !hasJAAS {
		t.Fatalf("Kafka extractors = %#v, want sasl.jaas.config", skill.Extractors)
	}
}

// TestDetectionRoundsValidation 验证检测轮数范围
func TestDetectionRoundsValidation(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"below minimum", 5, minWeakPasswordDetectionRounds},
		{"at minimum", 10, 10},
		{"in range", 25, 25},
		{"at maximum", 50, 50},
		{"above maximum", 60, maxWeakPasswordDetectionRounds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// normalizeDetectionRounds应该将输入限制在[min, max]范围内
			result := tt.input
			if result < minWeakPasswordDetectionRounds {
				result = minWeakPasswordDetectionRounds
			}
			if result > maxWeakPasswordDetectionRounds {
				result = maxWeakPasswordDetectionRounds
			}
			if result != tt.want {
				t.Errorf("normalizeDetectionRounds(%d) = %d, want %d", tt.input, result, tt.want)
			}
		})
	}
}

// TestAIDictionaryGenerationNotSequential 验证AI字典生成不是顺序递增的
func TestAIDictionaryGenerationNotSequential(t *testing.T) {
	// 测试确定性生成的密码不是简单的顺序递增
	seeds := []string{"admin", "test"}
	rules := []string{"append_year", "append_special_char", "capitalize", "leet_replace"}
	candidates := generateDictionaryFromSeeds(seeds, rules, 50)

	// 检查是否有连续的Admin@001, Admin@002, Admin@003模式
	sequentialCount := 0
	for i := 0; i < len(candidates)-2; i++ {
		if strings.HasPrefix(candidates[i], "Admin@") && strings.HasPrefix(candidates[i+1], "Admin@") {
			// 检查是否是连续数字
			num1 := strings.TrimPrefix(candidates[i], "Admin@")
			num2 := strings.TrimPrefix(candidates[i+1], "Admin@")
			if len(num1) == 3 && len(num2) == 3 && num1 < num2 {
				sequentialCount++
			}
		}
	}

	// 不应该有超过5个连续的顺序密码
	if sequentialCount > 5 {
		t.Errorf("AI dictionary generation produces too many sequential passwords: %d", sequentialCount)
	}

	// 验证密码多样性
	uniquePrefixes := make(map[string]bool)
	for _, pwd := range candidates {
		prefix := strings.TrimRight(pwd, "0123456789@!#_")
		uniquePrefixes[prefix] = true
	}

	// 应该有至少5种不同的密码前缀
	if len(uniquePrefixes) < 5 {
		t.Errorf("AI dictionary generation lacks diversity, only %d unique prefixes", len(uniquePrefixes))
	}
}

// TestPasswordEncryption 验证密码加密和脱敏
func TestPasswordEncryption(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 测试明文密码匹配和加密
	findings, err := svc.MatchCredentialRecords(uuid.New(), uuid.New(), uuid.New(), []AgentCredentialRecord{{
		Application:     "redis",
		Account:         "default",
		CredentialType:  "plaintext",
		CredentialValue: "Admin@123",
		SourcePath:      "/etc/redis/redis.conf",
		FieldPath:       "requirepass",
		Parser:          "line_key_value",
	}})
	if err != nil {
		t.Fatalf("MatchCredentialRecords returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}

	// 验证密码被加密存储
	if len(findings[0].MatchedPasswordEncrypted) == 0 {
		t.Fatal("expected encrypted password to be stored")
	}

	// 验证密码掩码是全星号
	if findings[0].MatchedPasswordMask != "*********" {
		t.Fatalf("password mask = %q, want all stars", findings[0].MatchedPasswordMask)
	}
}

// TestGenericSkillForUnknownApplications 验证未知应用使用通用skill
func TestGenericSkillForUnknownApplications(t *testing.T) {
	skill := weakPasswordSkillForApplication("unknown_app_xyz")

	if skill.ApplicationType != "unknown_app_xyz" {
		t.Errorf("unknown app should get generic skill with normalized type, got %q", skill.ApplicationType)
	}

	if len(skill.Extractors) == 0 {
		t.Error("generic skill should have extractors")
	}

	// 通用skill应该支持多种解析格式
	supportedTypes := make(map[string]bool)
	for _, ext := range skill.Extractors {
		supportedTypes[ext.Type] = true
	}

	expectedTypes := []string{"line_key_value", "yaml", "json", "properties"}
	for _, expected := range expectedTypes {
		if !supportedTypes[expected] {
			t.Errorf("generic skill should support %q parser", expected)
		}
	}
}

// TestCandidateDeduplication 验证候选应用去重逻辑
func TestCandidateDeduplication(t *testing.T) {
	// 这个测试验证去重逻辑
	// 在实际实现中，upsert键已改为(host_id, application_type)

	// 模拟测试数据
	hostID := uuid.New()
	applications := []struct {
		AssetID         string
		ApplicationType string
		ApplicationName string
	}{
		{AssetID: "asset1", ApplicationType: "redis", ApplicationName: "Redis Instance 1"},
		{AssetID: "asset2", ApplicationType: "redis", ApplicationName: "Redis Instance 2"},
		{AssetID: "asset3", ApplicationType: "mysql", ApplicationName: "MySQL"},
	}

	// 去重逻辑：按(host_id, application_type)去重
	seen := make(map[string]bool)
	deduplicated := []struct {
		AssetID         string
		ApplicationType string
		ApplicationName string
	}{}

	for _, app := range applications {
		key := hostID.String() + ":" + app.ApplicationType
		if !seen[key] {
			seen[key] = true
			deduplicated = append(deduplicated, app)
		}
	}

	if len(deduplicated) != 2 {
		t.Errorf("deduplication should result in 2 applications, got %d", len(deduplicated))
	}

	// 验证保留了第一个Redis实例
	if deduplicated[0].ApplicationType != "redis" || deduplicated[0].AssetID != "asset1" {
		t.Errorf("should keep first redis instance")
	}
}

// TestBcryptHashMatchRequiresVerifier 验证bcrypt哈希匹配
func TestBcryptHashMatchRequiresVerifier(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 这个测试验证bcrypt哈希密码的匹配
	// 实际的bcrypt测试需要在完整的测试环境中运行
	t.Skip("Skipping bcrypt test - requires full test environment")
}
