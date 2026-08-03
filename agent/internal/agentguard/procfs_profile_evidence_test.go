package agentguard

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoverConfigEvidenceCoversAllBuiltinProfiles(t *testing.T) {
	home := t.TempDir()
	for _, marker := range []string{
		".codex", ".openclaw", ".hermes", ".claude", ".config/opencode", ".gemini",
	} {
		if err := os.MkdirAll(filepath.Join(home, marker), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	got := discoverConfigEvidenceInHome(home)
	for _, marker := range []string{
		".codex", ".openclaw", ".hermes", ".claude", ".config/opencode", ".gemini",
	} {
		if !slices.Contains(got, marker) {
			t.Fatalf("config evidence missing %q: %v", marker, got)
		}
	}
}
