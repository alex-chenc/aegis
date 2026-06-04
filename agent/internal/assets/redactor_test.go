package assets

import (
	"strings"
	"testing"
)

func TestRedactCmdlineMasksSensitiveValues(t *testing.T) {
	cmdline := "nginx --password secret123 --token=abc AWS_SECRET_ACCESS_KEY=topsecret https://user:pass@example.com"

	redacted := RedactCmdline(cmdline)

	for _, secret := range []string{"secret123", "abc", "topsecret", "pass@example.com"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted cmdline still contains %q: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "***") {
		t.Fatalf("expected masked value in %q", redacted)
	}
}

func TestRedactConfigSummaryMasksSensitiveValues(t *testing.T) {
	config := "password = hunter2\napi_key: key-123\naccess_key=ak-123\nlisten=0.0.0.0"

	redacted := RedactConfigSummary(config)

	for _, secret := range []string{"hunter2", "key-123", "ak-123"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted config still contains %q: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "listen=0.0.0.0") {
		t.Fatalf("non-sensitive config was unexpectedly removed: %s", redacted)
	}
}
