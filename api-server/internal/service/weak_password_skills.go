package service

import (
	"strings"

	"api-server/internal/model"
)

type WeakPasswordSkill struct {
	ID              string
	ProfileID       string
	ApplicationType string
	Aliases         []string
	CandidatePaths  []string
	CredentialTypes []string
	Extractors      []CredentialExtractor
	Reason          string
}

var weakPasswordSkillRegistry = []WeakPasswordSkill{
	{
		ID:              "redis_fixed_config_v1",
		ProfileID:       "redis_config_v1",
		ApplicationType: "redis",
		Aliases:         []string{"redis", "redis-server"},
		CandidatePaths: []string{
			"/etc/redis/redis.conf",
			"/etc/redis.conf",
			"/usr/local/etc/redis/redis.conf",
			"/data/redis.conf",
			"/redis.conf",
		},
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeAuthString},
		Extractors: []CredentialExtractor{
			{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext},
			{Type: "line_key_value", PasswordSelector: "masterauth", FormatHint: model.CredTypePlaintext},
		},
		Reason: "Redis 固定 skill：读取 redis.conf 中 requirepass/masterauth",
	},
	{
		ID:              "openssh_shadow_v1",
		ProfileID:       "openssh_shadow_v1",
		ApplicationType: "openssh",
		Aliases:         []string{"ssh", "sshd", "openssh", "linux_shadow"},
		CandidatePaths:  []string{"/etc/shadow"},
		CredentialTypes: []string{model.CredTypeSaltedHash},
		Extractors: []CredentialExtractor{
			{Type: "shadow", SourceKind: "system_account", FormatHint: model.CredTypeSaltedHash},
		},
		Reason: "OpenSSH 固定 skill：读取 /etc/shadow 的账号、salt 和 hash",
	},
	{
		ID:              "mysql_config_v1",
		ProfileID:       "mysql_config_v1",
		ApplicationType: "mysql",
		Aliases:         []string{"mysql", "mariadb"},
		CandidatePaths: []string{
			"/etc/mysql/my.cnf",
			"/etc/my.cnf",
			"/usr/local/etc/my.cnf",
			"/etc/mysql/mysql.conf.d/mysqld.cnf",
			"/etc/mysql/mariadb.conf.d/50-server.cnf",
			"/root/.my.cnf",
			"/etc/mysql/debian.cnf",
		},
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeAuthString, model.CredTypeSaltedHash},
		Extractors: []CredentialExtractor{
			{Type: "ini", Section: "client", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
			{Type: "ini", Section: "client", AccountSelector: "user", PasswordSelector: "password1", FormatHint: model.CredTypePlaintext},
		},
		Reason: "MySQL/MariaDB 固定 skill：优先解析常见客户端配置中的账号密码",
	},
	{
		ID:              "postgres_config_v1",
		ProfileID:       "postgres_config_v1",
		ApplicationType: "postgresql",
		Aliases:         []string{"postgres", "postgresql"},
		CandidatePaths: []string{
			"/var/lib/postgresql/.pgpass",
			"/root/.pgpass",
			"/var/lib/postgresql/data/postgresql.conf",
			"/etc/postgresql/postgresql.conf",
		},
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeAuthString},
		Extractors: []CredentialExtractor{
			{Type: "properties", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
			{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
		},
		Reason: "PostgreSQL 固定 skill：解析 .pgpass 或配置中明文凭据",
	},
	{
		ID:              "tomcat_users_v1",
		ProfileID:       "tomcat_users_v1",
		ApplicationType: "tomcat",
		Aliases:         []string{"tomcat", "catalina"},
		CandidatePaths: []string{
			"/usr/local/tomcat/conf/tomcat-users.xml",
			"/opt/tomcat/conf/tomcat-users.xml",
			"/etc/tomcat/tomcat-users.xml",
			"/etc/tomcat8/tomcat-users.xml",
			"/etc/tomcat9/tomcat-users.xml",
			"/etc/tomcat10/tomcat-users.xml",
		},
		CredentialTypes: []string{model.CredTypePlaintext},
		Extractors: []CredentialExtractor{
			{Type: "tomcat_users_xml", AccountSelector: "username", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
		},
		Reason: "Tomcat 固定 skill：解析 tomcat-users.xml 的用户密码",
	},
	{
		ID:              "ftp_config_v1",
		ProfileID:       "ftp_config_v1",
		ApplicationType: "ftp",
		Aliases:         []string{"ftp", "vsftpd", "proftpd"},
		CandidatePaths: []string{
			"/etc/vsftpd/virtual_users.db",
			"/etc/proftpd/passwd",
			"/etc/proftpd/ftppasswd",
			"/etc/proftpd/ftpd.passwd",
			"/etc/shadow",
		},
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeSaltedHash},
		Extractors: []CredentialExtractor{
			{Type: "shadow", SourceKind: "system_account", FormatHint: model.CredTypeSaltedHash},
			{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
		},
		Reason: "FTP 固定 skill：兼容 proftpd 类 shadow 文件和虚拟用户配置",
	},
	{
		ID:              "basic_auth_v1",
		ProfileID:       "basic_auth_v1",
		ApplicationType: "web_service",
		Aliases:         []string{"nginx", "apache", "httpd", "web_service"},
		CandidatePaths: []string{
			"/etc/nginx/.htpasswd",
			"/usr/local/nginx/conf/.htpasswd",
			"/etc/apache2/.htpasswd",
			"/etc/httpd/.htpasswd",
			"/etc/httpd/conf/.htpasswd",
		},
		CredentialTypes: []string{model.CredTypeHash, model.CredTypeSaltedHash},
		Extractors: []CredentialExtractor{
			{Type: "htpasswd", FormatHint: model.CredTypeHash},
		},
		Reason: "Web Basic Auth 固定 skill：解析 .htpasswd",
	},
	{
		ID:              "kafka_config_v1",
		ProfileID:       "kafka_config_v1",
		ApplicationType: "kafka",
		Aliases:         []string{"kafka", "apache kafka", "confluent-kafka"},
		CandidatePaths: []string{
			"/etc/kafka/kafka.properties",
			"/etc/kafka/server.properties",
			"/etc/kafka/kafka_server_jaas.conf",
			"/etc/kafka/kraft/server.properties",
			"/etc/kafka/kraft/broker.properties",
			"/etc/kafka/secrets/kafka_server_jaas.conf",
		},
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeAuthString},
		Extractors: []CredentialExtractor{
			{Type: "properties", PasswordSelector: "sasl.jaas.config", FormatHint: model.CredTypeAuthString},
			{Type: "properties", PasswordSelector: "sasl.password", FormatHint: model.CredTypePlaintext},
			{Type: "properties", PasswordSelector: "ssl.key.password", FormatHint: model.CredTypePlaintext},
			{Type: "properties", PasswordSelector: "ssl.keystore.password", FormatHint: model.CredTypePlaintext},
			{Type: "properties", PasswordSelector: "ssl.truststore.password", FormatHint: model.CredTypePlaintext},
		},
		Reason: "Kafka 固定 skill：解析 Kafka properties、JAAS 和 SSL 相关凭据字段",
	},
	{
		ID:              "llm_service_config_v1",
		ProfileID:       "llm_service_config_v1",
		ApplicationType: "llm_service",
		Aliases:         []string{"ai_agent", "mcp_server", "llm_service"},
		CandidatePaths:  nil,
		CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeAuthString},
		Extractors: []CredentialExtractor{
			{Type: "yaml", AccountSelector: "auth.user", PasswordSelector: "auth.token", FormatHint: model.CredTypeAuthString},
			{Type: "json", AccountSelector: "auth.user", PasswordSelector: "auth.token", FormatHint: model.CredTypeAuthString},
			{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "token", FormatHint: model.CredTypeAuthString},
		},
		Reason: "AI/MCP/LLM 服务固定 skill：解析常见 token/auth 配置",
	},
}

var weakPasswordGenericSkill = WeakPasswordSkill{
	ID:              "generic_config_v1",
	ProfileID:       "generic_config_v1",
	ApplicationType: "generic",
	CandidatePaths:  nil,
	CredentialTypes: []string{model.CredTypePlaintext, model.CredTypeUnknown},
	Extractors: []CredentialExtractor{
		{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
		{Type: "line_key_value", AccountSelector: "username", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
		{Type: "yaml", AccountSelector: "auth.user", PasswordSelector: "auth.password", FormatHint: model.CredTypePlaintext},
		{Type: "json", AccountSelector: "auth.user", PasswordSelector: "auth.password", FormatHint: model.CredTypePlaintext},
		{Type: "properties", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext},
	},
	Reason: "通用弱密码 skill：解析常见配置文件中的 user/password/token 字段",
}

func weakPasswordSkillForApplication(appType string) WeakPasswordSkill {
	normalized := normalizeApplicationType(appType)
	for _, skill := range weakPasswordSkillRegistry {
		for _, alias := range skill.Aliases {
			if normalized == normalizeApplicationType(alias) || strings.EqualFold(strings.TrimSpace(appType), alias) {
				return cloneWeakPasswordSkill(skill, normalized)
			}
		}
		if normalized == skill.ApplicationType {
			return cloneWeakPasswordSkill(skill, normalized)
		}
	}
	return cloneWeakPasswordSkill(weakPasswordGenericSkill, normalized)
}

func cloneWeakPasswordSkill(skill WeakPasswordSkill, normalizedAppType string) WeakPasswordSkill {
	out := skill
	if out.ApplicationType == "generic" && normalizedAppType != "" && normalizedAppType != "unknown" {
		out.ProfileID = normalizedAppType + "_config_v1"
		out.ApplicationType = normalizedAppType
	}
	out.Aliases = append([]string(nil), skill.Aliases...)
	out.CandidatePaths = append([]string(nil), skill.CandidatePaths...)
	out.CredentialTypes = append([]string(nil), skill.CredentialTypes...)
	out.Extractors = append([]CredentialExtractor(nil), skill.Extractors...)
	return out
}
