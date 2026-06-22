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
