package llm

import (
	"regexp"
	"strings"
	"testing"
)

func TestStaticLLMPromptsAreEnglish(t *testing.T) {
	han := regexp.MustCompile(`[\p{Han}]`)
	for name, prompt := range map[string]string{
		"rule_extraction":     RuleExtractionPrompt,
		"check_script":        CheckScriptGenerationPrompt,
		"fix_script":          FixScriptGenerationPrompt,
		"self_healing":        SelfHealingFixPrompt,
		"cve_analysis":        CVEAnalysisPrompt,
		"cve_analysis_legacy": CVEAnalysisPromptZH,
		"vulnerability_fix":   VulnerabilityFixPrompt,
		"poc_verification":    POCVerificationPrompt,
		"react":               ReActPromptTemplate,
		"script_audit":        ScriptAuditSystemPrompt,
		"detection_package":   DetectionPackageGenerationPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			if han.MatchString(prompt) {
				t.Fatalf("%s prompt contains Han characters:\n%s", name, prompt)
			}
		})
	}
}

func TestFixScriptGenerationPromptIncludesCheckAndFixLogic(t *testing.T) {
	prompt := GetFixScriptGenerationPrompt("检查 sshd PermitRootLogin 必须为 no", "将 PermitRootLogin 设置为 no")

	for _, want := range []string{
		"检查 sshd PermitRootLogin 必须为 no",
		"将 PermitRootLogin 设置为 no",
		"inverse of the check",
		"post-remediation verification",
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
		"non-compliant",
		"exit code 2",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}
