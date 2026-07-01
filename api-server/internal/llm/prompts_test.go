package llm

import (
	"strings"
	"testing"
)

func TestFixScriptGenerationPromptIncludesCheckAndFixLogic(t *testing.T) {
	prompt := GetFixScriptGenerationPrompt("检查 sshd PermitRootLogin 必须为 no", "将 PermitRootLogin 设置为 no")

	for _, want := range []string{
		"检查 sshd PermitRootLogin 必须为 no",
		"将 PermitRootLogin 设置为 no",
		"检测内容的逆逻辑",
		"验证步骤",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestCheckScriptGenerationPromptDefinesNonCompliantExitCode(t *testing.T) {
	prompt := GetCheckScriptGenerationPrompt("检查 sshd PermitRootLogin 必须为 no")

	for _, want := range []string{
		"exit code 0",
		"exit code 1",
		"基线项未通过",
		"exit code 2",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}
