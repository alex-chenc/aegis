package blocker

import (
	"strings"
	"testing"
)

func TestExecuteReturnsReadableFailureReasons(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		target  string
		wantErr string
	}{
		{
			name:    "missing quarantine target",
			action:  "quarantine_file",
			target:  "",
			wantErr: "missing target for quarantine_file",
		},
		{
			name:    "invalid network target",
			action:  "block_connection",
			target:  "not-an-ip",
			wantErr: "invalid remote address for block_connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewBlocker(t.TempDir()).Execute(tt.action, tt.target)
			if err == nil {
				t.Fatal("expected failure")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
