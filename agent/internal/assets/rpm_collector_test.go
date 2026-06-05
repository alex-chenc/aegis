package assets

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestParseRPMLineParsesUnixInstallTime(t *testing.T) {
	collector := NewRPMCollector(zap.NewNop())

	pkg, err := collector.parseRPMLine("nginx|(none)|1.24.0|1.el9|x86_64|1700000000|nginx-1.24.0.src.rpm|BSD|Example")
	if err != nil {
		t.Fatalf("expected rpm line to parse: %v", err)
	}

	want := time.Unix(1700000000, 0)
	if !pkg.InstallTime.Equal(want) {
		t.Fatalf("expected install time %s, got %s", want, pkg.InstallTime)
	}
}
