package llm

import (
	"strings"
	"testing"

	"baseline-system/pkg/logger"
)

func init() {
	_ = logger.Init(&logger.Config{Level: "info"})
}

func TestParseScript_AddsShebang(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantShebang bool
		wantErr     bool
	}{
		{
			name:        "script without shebang gets one added",
			input:       "echo 'hello world'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "script with shebang preserved",
			input:       "#!/bin/bash\necho 'hello'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "script with different shebang preserved",
			input:       "#!/bin/sh\necho 'hello'",
			wantShebang: true,
			wantErr:     false,
		},
		{
			name:        "empty script returns error",
			input:       "",
			wantShebang: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseScript(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for empty script")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(result, "#!") {
				t.Errorf("script should start with shebang, got: %s", result[:min(20, len(result))])
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
