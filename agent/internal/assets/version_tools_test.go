package assets

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestExtractVersionHandlesSlashSeparatedOutput(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	got := tool.extractVersion("nginx version: nginx/1.24.0")
	if got != "1.24.0" {
		t.Fatalf("expected nginx version 1.24.0, got %q", got)
	}
}

func TestExtractVersionHandlesVPrefix(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	got := tool.extractVersion("v20.11.1")
	if got != "20.11.1" {
		t.Fatalf("expected node version 20.11.1, got %q", got)
	}
}

func TestExtractVersionHandlesOpenSSHPatchVersion(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	got := tool.extractVersion("OpenSSH_10.0p2 Ubuntu-5ubuntu5.4, OpenSSL 3.5.3")
	if got != "10.0p2" {
		t.Fatalf("expected OpenSSH version 10.0p2, got %q", got)
	}
}

func TestExtractVersionHandlesMihomoOutput(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	got := tool.extractVersion("Mihomo Meta v1.19.25 linux amd64")
	if got != "1.19.25" {
		t.Fatalf("expected Mihomo version 1.19.25, got %q", got)
	}
}

func TestAssetReadProcFileRejectsSensitivePathVariants(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	result := tool.AssetReadProcFile(context.Background(), os.Getpid(), "./environ")
	if result.Success {
		t.Fatalf("expected ./environ to be rejected, got %#v", result)
	}
	if !strings.Contains(result.Error, "forbidden") {
		t.Fatalf("expected forbidden error, got %q", result.Error)
	}
}

func TestAssetReadProcFileReadsStatus(t *testing.T) {
	tool := NewVersionTool(zap.NewNop())

	result := tool.AssetReadProcFile(context.Background(), os.Getpid(), "status")
	if !result.Success {
		t.Fatalf("expected status to be readable: %#v", result)
	}
	if !strings.Contains(result.Content, "Name:") {
		t.Fatalf("expected proc status content, got %q", result.Content[:minInt(len(result.Content), 80)])
	}
	if result.Truncated {
		t.Fatal("did not expect status to be truncated")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
