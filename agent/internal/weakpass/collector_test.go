package weakpass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectCredentialsParsesShadow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shadow")
	content := "root:$6$saltvalue$hashvalue:19400:0:99999:7:::\nlocked:!:19400:0:99999:7:::\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := NewCollector(nil)
	result, err := collector.CollectCredentials(t.Context(), map[string]interface{}{
		"task_id": "task",
		"plan_id": "plan",
		"host_id": "host",
		"applications": []map[string]interface{}{{
			"application": "linux_shadow",
			"paths":       []string{path},
			"extractors": []map[string]interface{}{{
				"type":        "shadow",
				"format_hint": "salted_hash",
			}},
		}},
		"collection_policy": map[string]interface{}{
			"max_file_bytes":          1024,
			"max_records":             10,
			"forbid_find_command":     true,
			"forbid_recursive_search": true,
		},
	})
	if err != nil {
		t.Fatalf("CollectCredentials returned error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("records = %d, want 1; errors=%v", len(result.Records), result.Errors)
	}
	record := result.Records[0]
	if record.Account != "root" || record.Salt != "saltvalue" || record.AlgorithmHint != "sha512-crypt" {
		t.Fatalf("unexpected record: %#v", record)
	}
}

func TestCollectCredentialsParsesRedisRequirepass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.conf")
	if err := os.WriteFile(path, []byte("bind 127.0.0.1\nrequirepass Redis@123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := NewCollector(nil)
	result, err := collector.CollectCredentials(t.Context(), baseCollectParams(path, "redis", "line_key_value", "", "requirepass"))
	if err != nil {
		t.Fatalf("CollectCredentials returned error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("records = %d, want 1; errors=%v", len(result.Records), result.Errors)
	}
	if result.Records[0].CredentialValue != "Redis@123" || result.Records[0].CredentialType != CredentialTypePlaintext {
		t.Fatalf("unexpected record: %#v", result.Records[0])
	}
}

func TestCollectCredentialsParsesTomcatUsersXML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tomcat-users.xml")
	content := `<tomcat-users>
  <user username="tomcat" password="Tomcat@123" roles="manager-gui"/>
</tomcat-users>`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := NewCollector(nil)
	result, err := collector.CollectCredentials(t.Context(), baseCollectParams(path, "tomcat", "tomcat_users_xml", "username", "password"))
	if err != nil {
		t.Fatalf("CollectCredentials returned error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("records = %d, want 1; errors=%v", len(result.Records), result.Errors)
	}
	if result.Records[0].Account != "tomcat" || result.Records[0].CredentialValue != "Tomcat@123" {
		t.Fatalf("unexpected record: %#v", result.Records[0])
	}
}

func TestCredentialPathCandidatesIncludeProcRootForRelatedPIDs(t *testing.T) {
	candidates := credentialPathCandidatesWithResolver("/etc/redis/redis.conf", []int{1234, 1234, 0}, false, func(pid int) (string, bool) {
		if pid == 1234 {
			return "/proc/1234/root", true
		}
		return "", false
	})
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(candidates))
	}
	if candidates[0].ReadPath != "/proc/1234/root/etc/redis/redis.conf" {
		t.Fatalf("first candidate path = %q, want proc root", candidates[0].ReadPath)
	}
	if candidates[0].SourcePath != "/proc/1234/root/etc/redis/redis.conf" || candidates[0].ProcessPID != 1234 {
		t.Fatalf("unexpected container candidate: %#v", candidates[0])
	}
	if candidates[1].ReadPath != "/etc/redis/redis.conf" || candidates[1].ProcessPID != 0 {
		t.Fatalf("unexpected host fallback candidate: %#v", candidates[1])
	}
}

func TestCredentialPathCandidatesDoNotUseProcRootForNonContainerPID(t *testing.T) {
	candidates := credentialPathCandidatesWithResolver("/etc/redis/redis.conf", []int{1234}, false, func(pid int) (string, bool) {
		return "", false
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want host fallback only", len(candidates))
	}
	if candidates[0].ReadPath != "/etc/redis/redis.conf" || candidates[0].ProcessPID != 0 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestCredentialPathCandidatesContainerOnlySkipsHostFallback(t *testing.T) {
	candidates := credentialPathCandidatesWithResolver("/etc/kafka/server.properties", []int{6937}, true, func(pid int) (string, bool) {
		if pid == 6937 {
			return "/proc/6937/root", true
		}
		return "", false
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want only container path", len(candidates))
	}
	if candidates[0].ReadPath != "/proc/6937/root/etc/kafka/server.properties" || candidates[0].SourcePath != candidates[0].ReadPath {
		t.Fatalf("unexpected container-only candidate: %#v", candidates[0])
	}
}

func TestRedisCredentialRecordsFromCmdline(t *testing.T) {
	app := ApplicationCollectPlan{Application: "redis", AssetID: "asset-1"}
	records := redisCredentialRecordsFromCmdline(app, 4321, []string{
		"redis-server",
		"--requirepass",
		"Redis@123",
		"--masterauth=Master@123",
	})
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].CredentialValue != "Redis@123" || records[0].FieldPath != "cmdline.requirepass" || records[0].ProcessPID != 4321 {
		t.Fatalf("unexpected requirepass record: %#v", records[0])
	}
	if records[1].CredentialValue != "Master@123" || records[1].FieldPath != "cmdline.masterauth" {
		t.Fatalf("unexpected masterauth record: %#v", records[1])
	}
}

func TestParseDockerContainerCommandParts(t *testing.T) {
	parts, err := parseDockerContainerCommandParts([]byte(`{
		"Path": "docker-entrypoint.sh",
		"Args": ["redis-server", "--requirepass", "Redis@123"],
		"Config": {
			"Entrypoint": ["ignored-entrypoint"],
			"Cmd": ["ignored-cmd"]
		}
	}`))
	if err != nil {
		t.Fatalf("parseDockerContainerCommandParts returned error: %v", err)
	}
	if len(parts) != 3 || parts[0] != "redis-server" || parts[2] != "Redis@123" {
		t.Fatalf("parts = %#v, want Args command", parts)
	}

	parts, err = parseDockerContainerCommandParts([]byte(`{
		"Config": {
			"Entrypoint": ["docker-entrypoint.sh"],
			"Cmd": ["redis-server", "--masterauth=Master@123"]
		}
	}`))
	if err != nil {
		t.Fatalf("parseDockerContainerCommandParts returned error: %v", err)
	}
	if len(parts) != 3 || parts[0] != "docker-entrypoint.sh" || parts[2] != "--masterauth=Master@123" {
		t.Fatalf("parts = %#v, want Entrypoint + Cmd command", parts)
	}
}

func TestDockerContainerEnvCredentialRecords(t *testing.T) {
	app := ApplicationCollectPlan{Application: "redis", AssetID: "asset-1"}
	env, err := parseDockerContainerEnv([]byte(`{
		"Config": {
			"Env": [
				"PATH=/usr/local/bin:/usr/bin",
				"REDIS_PASSWORD=EnvRedis@123",
				"REDISCLI_AUTH=CliAuth@123"
			]
		}
	}`))
	if err != nil {
		t.Fatalf("parseDockerContainerEnv returned error: %v", err)
	}
	records := credentialRecordsFromEnv(app, 4321, env, "/var/lib/docker/containers/container-id/config.v2.json")
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].CredentialValue != "EnvRedis@123" || records[0].FieldPath != "Env.REDIS_PASSWORD" || records[0].SourceKind != "container_env" {
		t.Fatalf("unexpected env record: %#v", records[0])
	}
	if records[1].CredentialValue != "CliAuth@123" || records[1].FieldPath != "Env.REDISCLI_AUTH" {
		t.Fatalf("unexpected env auth record: %#v", records[1])
	}
}

func TestRedisCredentialRecordsFromDockerConfigArgs(t *testing.T) {
	app := ApplicationCollectPlan{Application: "redis", AssetID: "asset-1"}
	records := redisCredentialRecordsFromArgs(app, 4321, []string{
		"redis-server",
		"--requirepass",
		"Redis@123",
	}, "/var/lib/docker/containers/container-id/config.v2.json", "container_runtime_config", "docker_config_cmd", "docker_config", 0.9)

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.CredentialValue != "Redis@123" || record.SourceKind != "container_runtime_config" || record.Parser != "docker_config_cmd" {
		t.Fatalf("unexpected docker config record: %#v", record)
	}
	if record.FieldPath != "docker_config.requirepass" || record.ProcessPID != 4321 {
		t.Fatalf("unexpected docker config location: %#v", record)
	}
}

func TestCollectCredentialsParsesMySQLIni(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debian.cnf")
	if err := os.WriteFile(path, []byte("[client]\nuser = debian-sys-maint\npassword = Mysql@123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	params := baseCollectParams(path, "mysql", "ini", "user", "password")
	app := params["applications"].([]map[string]interface{})[0]
	extractor := app["extractors"].([]map[string]interface{})[0]
	extractor["section"] = "client"

	collector := NewCollector(nil)
	result, err := collector.CollectCredentials(t.Context(), params)
	if err != nil {
		t.Fatalf("CollectCredentials returned error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("records = %d, want 1; errors=%v", len(result.Records), result.Errors)
	}
	if result.Records[0].Account != "debian-sys-maint" || result.Records[0].CredentialValue != "Mysql@123" {
		t.Fatalf("unexpected record: %#v", result.Records[0])
	}
}

func TestCollectCredentialsRejectsUnsafePathAndShellText(t *testing.T) {
	collector := NewCollector(nil)
	_, err := collector.CollectCredentials(t.Context(), baseCollectParams("/tmp/a;find /", "redis", "line_key_value", "", "requirepass"))
	if err == nil || !strings.Contains(err.Error(), "forbidden command text") {
		t.Fatalf("expected forbidden command error, got %v", err)
	}
}

func TestListConfigDirRejectsRecursive(t *testing.T) {
	collector := NewCollector(nil)
	_, err := collector.ListConfigDir(t.Context(), map[string]interface{}{
		"dir":       t.TempDir(),
		"recursive": true,
	})
	if err == nil || !strings.Contains(err.Error(), ErrRecursiveNotAllowed) {
		t.Fatalf("expected recursive rejection, got %v", err)
	}
}

func TestReadConfigSliceRedactsSensitiveValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.properties")
	if err := os.WriteFile(path, []byte("username=admin\npassword=Admin@123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := NewCollector(nil)
	result, err := collector.ReadConfigSlice(t.Context(), map[string]interface{}{
		"path":          path,
		"redact_values": true,
	})
	if err != nil {
		t.Fatalf("ReadConfigSlice returned error: %v", err)
	}
	joined := strings.Join(result.Lines, "\n")
	if strings.Contains(joined, "Admin@123") || !strings.Contains(joined, "password = ******") {
		t.Fatalf("expected redacted output, got %q", joined)
	}
}

func TestParseContainerIdentityFromCgroupSupportsCommonRuntimes(t *testing.T) {
	fullID := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	cases := []struct {
		name    string
		content string
		runtime string
		id      string
	}{
		{
			name:    "docker systemd scope",
			content: "0::/system.slice/docker-" + fullID + ".scope\n",
			runtime: "docker",
			id:      fullID,
		},
		{
			name:    "containerd kubepods scope",
			content: "0::/kubepods.slice/kubepods-besteffort.slice/cri-containerd-" + fullID + ".scope\n",
			runtime: "containerd",
			id:      fullID,
		},
		{
			name:    "crio scope",
			content: "0::/kubepods/burstable/crio-" + fullID + ".scope\n",
			runtime: "cri-o",
			id:      fullID,
		},
		{
			name:    "podman libpod scope",
			content: "0::/user.slice/libpod-" + fullID + ".scope\n",
			runtime: "podman",
			id:      fullID,
		},
		{
			name:    "docker slash short id",
			content: "12:memory:/docker/1234567890ab\n",
			runtime: "docker",
			id:      "1234567890ab",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			identity := parseContainerIdentityFromCgroup(tt.content)
			if identity.Runtime != tt.runtime || identity.ID != tt.id {
				t.Fatalf("identity = %#v, want runtime=%s id=%s", identity, tt.runtime, tt.id)
			}
		})
	}

	if identity := parseContainerIdentityFromCgroup("0::/\n"); identity.ID != "" {
		t.Fatalf("expected no container identity, got %#v", identity)
	}
}

func TestConfigPathsFromCmdlineExtractsConfigFlags(t *testing.T) {
	paths := configPathsFromCmdline([]string{
		"redis-server",
		"--requirepass",
		"Secret@123",
		"--config=/etc/redis/redis.conf",
		"--include",
		"ignored",
	}, "/srv/redis", []string{".conf"})

	if len(paths) != 1 || paths[0] != "/etc/redis/redis.conf" {
		t.Fatalf("paths = %#v, want /etc/redis/redis.conf", paths)
	}

	paths = configPathsFromCmdline([]string{"redis-server", "redis.conf"}, "/srv/redis", []string{".conf"})
	if len(paths) != 1 || paths[0] != "/srv/redis/redis.conf" {
		t.Fatalf("relative paths = %#v, want /srv/redis/redis.conf", paths)
	}

	paths = configPathsFromCmdline([]string{
		"java",
		"-Dlog4j.configuration=file:/etc/kafka/log4j.properties",
		"-Djava.security.auth.login.config=/etc/kafka/kafka_server_jaas.conf",
	}, "/home/appuser", []string{".conf", ".properties"})
	if !testContainsString(paths, "/etc/kafka/log4j.properties") || !testContainsString(paths, "/etc/kafka/kafka_server_jaas.conf") {
		t.Fatalf("jvm paths = %#v, want kafka file URI and JAAS config", paths)
	}
	for _, path := range paths {
		if strings.Contains(path, "-D") || strings.HasPrefix(path, "/home/appuser/-D") {
			t.Fatalf("jvm option was treated as a relative path: %#v", paths)
		}
	}
}

func TestRedactCmdlinePartsRedactsSeparatedRedisPassword(t *testing.T) {
	parts := redactCmdlineParts([]string{"redis-server", "--requirepass", "Redis@123", "--masterauth=Master@123"})
	joined := strings.Join(parts, " ")
	if strings.Contains(joined, "Redis@123") || strings.Contains(joined, "Master@123") {
		t.Fatalf("cmdline leaked secret: %q", joined)
	}
	if parts[2] != "******" || parts[3] != "--masterauth=******" {
		t.Fatalf("unexpected redacted parts: %#v", parts)
	}
}

func TestDiscoverContainerConfigPathsUsesProcRoot(t *testing.T) {
	root := t.TempDir()
	confPath := filepath.Join(root, "etc", "redis", "redis.conf")
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(confPath, []byte("requirepass Redis@123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	paths := discoverContainerConfigPaths(root, "redis", []string{".conf"}, 20, nil, nil)
	if len(paths) != 1 || paths[0] != "/etc/redis/redis.conf" {
		t.Fatalf("paths = %#v, want container source path", paths)
	}
}

func TestDiscoverContainerConfigPathsIncludesShadowWithoutSuffix(t *testing.T) {
	root := t.TempDir()
	shadowPath := filepath.Join(root, "etc", "shadow")
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shadowPath, []byte("root:$6$salt$hash:19400:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	paths := discoverContainerConfigPaths(root, "openssh", []string{".conf"}, 20, nil, nil)
	if len(paths) != 1 || paths[0] != "/etc/shadow" {
		t.Fatalf("paths = %#v, want /etc/shadow", paths)
	}
}

func TestDiscoverContainerConfigPathsUsesKafkaDirsOnly(t *testing.T) {
	root := t.TempDir()
	kafkaPath := filepath.Join(root, "etc", "kafka", "server.properties")
	genericPath := filepath.Join(root, "etc", "yum.conf")
	if err := os.MkdirAll(filepath.Dir(kafkaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kafkaPath, []byte("broker.id=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(genericPath, []byte("metadata_expire=48h\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	paths := discoverContainerConfigPaths(root, "kafka", []string{".conf", ".properties"}, 20, nil, nil)
	if !testContainsString(paths, "/etc/kafka/server.properties") {
		t.Fatalf("paths = %#v, want kafka server.properties", paths)
	}
	if testContainsString(paths, "/etc/yum.conf") {
		t.Fatalf("paths = %#v, should not include generic /etc files for kafka", paths)
	}
}

func baseCollectParams(path, app, parser, accountSelector, passwordSelector string) map[string]interface{} {
	return map[string]interface{}{
		"task_id": "task",
		"plan_id": "plan",
		"host_id": "host",
		"applications": []map[string]interface{}{{
			"application": app,
			"paths":       []string{path},
			"extractors": []map[string]interface{}{{
				"type":              parser,
				"account_selector":  accountSelector,
				"password_selector": passwordSelector,
				"format_hint":       "plaintext",
			}},
		}},
		"collection_policy": map[string]interface{}{
			"max_file_bytes":          1024,
			"max_records":             10,
			"forbid_find_command":     true,
			"forbid_recursive_search": true,
		},
	}
}

func testContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
