package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"api-server/internal/model"
	"api-server/internal/recovery"
)

type fakeDetectionPackageGenerationConfigRepo struct{}

func (fakeDetectionPackageGenerationConfigRepo) GetActive() (*model.LLMConfig, error) {
	return &model.LLMConfig{
		APIKeyEncrypted: "encrypted",
		BaseURL:         "https://llm.invalid",
		ModelName:       "test-model",
	}, nil
}

func (fakeDetectionPackageGenerationConfigRepo) DecryptAPIKey(string) (string, error) {
	return "decrypted", nil
}

type fakeDetectionPackageDraftCreator struct {
	allowlist   *AllowlistConfig
	request     CreateDraftRequest
	operator    string
	createCount int
}

func (f *fakeDetectionPackageDraftCreator) CreateDraft(_ context.Context, req CreateDraftRequest, operator string) (*model.DetectionPackageDraft, error) {
	f.request = req
	f.operator = operator
	f.createCount++
	return &model.DetectionPackageDraft{
		PackageID: req.PackageID,
		Status:    "draft",
	}, nil
}

func (f *fakeDetectionPackageDraftCreator) GetActiveHookAllowlist(context.Context) (*AllowlistConfig, error) {
	return f.allowlist, nil
}

type fakeDetectionPackageLLM struct {
	responses []string
	prompts   []string
	calls     int
}

func (f *fakeDetectionPackageLLM) ChatCompletion(_ context.Context, _, prompt string, _ float64) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.calls >= len(f.responses) {
		return "", nil
	}
	response := f.responses[f.calls]
	f.calls++
	return response, nil
}

func TestDetectionPackageGenerationServiceGeneratesAndPersistsCompleteDraft(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{detectionPackageGenerationResponse("syscalls/sys_enter_socket")}}
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	draft, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "cve-2026-31431",
		VulnerabilityDescription: "AF_ALG with pipe and splice modifies page cache.",
		ExploitationChain:        "socket -> pipe -> splice",
		Operator:                 "assistant-test",
	})
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if draft == nil || draft.PackageID == "" || draft.Status != "draft" {
		t.Fatalf("unexpected generated draft: %#v", draft)
	}
	if creator.request.CVEIDs[0] != "CVE-2026-31431" {
		t.Fatalf("CVE IDs were not normalized: %#v", creator.request.CVEIDs)
	}
	if creator.request.HookPlanYAML == "" || creator.request.EBPFSource == "" ||
		creator.request.SigmaRulesYAML == "" || creator.request.CorrelationYAML == "" {
		t.Fatalf("generated package content is incomplete: %#v", creator.request)
	}
	if creator.operator != "assistant-test" {
		t.Fatalf("operator = %q, want assistant-test", creator.operator)
	}
	if !creator.request.AIGenerated {
		t.Fatal("AI-generated draft was persisted without ai_generated provenance")
	}
	if creator.request.AIGenerationInput["cve_id"] != "CVE-2026-31431" {
		t.Fatalf("AI generation input was not persisted: %#v", creator.request.AIGenerationInput)
	}
}

func TestDetectionPackageGenerationServiceInjectsAllowlistAndCorrectsRejectedDraft(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{
		detectionPackageGenerationResponse("syscalls/sys_enter_socket"),
		detectionPackageGenerationResponse("syscalls/sys_enter_execve"),
	}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	draft, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "AF_ALG exploit chain",
	})
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if draft == nil || creator.createCount != 1 {
		t.Fatalf("corrected draft was not persisted exactly once: draft=%#v creates=%d", draft, creator.createCount)
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("LLM calls = %d, want one generation plus one correction", fakeLLM.calls)
	}
	if !strings.Contains(fakeLLM.prompts[0], "syscalls/sys_enter_execve") ||
		!strings.Contains(fakeLLM.prompts[0], "ACTIVE HOOK ALLOWLIST") {
		t.Fatalf("initial prompt does not contain the active hook contract: %q", fakeLLM.prompts[0])
	}
	if !strings.Contains(fakeLLM.prompts[1], "Previous draft rejected") ||
		!strings.Contains(fakeLLM.prompts[1], "syscalls/sys_enter_socket") {
		t.Fatalf("correction prompt does not contain the validation failure: %q", fakeLLM.prompts[1])
	}
	if !strings.Contains(creator.request.HookPlanYAML, "syscalls/sys_enter_execve") ||
		strings.Contains(creator.request.HookPlanYAML, "syscalls/sys_enter_socket") {
		t.Fatalf("persisted HookPlan did not use the corrected allowlisted hook: %q", creator.request.HookPlanYAML)
	}
}

func TestDetectionPackageGenerationServiceDoesNotPersistDraftRejectedAfterCorrection(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{
		detectionPackageGenerationResponse("syscalls/sys_enter_socket"),
		detectionPackageGenerationResponse("syscalls/sys_enter_bind"),
	}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	_, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "AF_ALG exploit chain",
	})
	if err == nil || !strings.Contains(err.Error(), "after one correction") {
		t.Fatalf("expected terminal allowlist rejection, got %v", err)
	}
	if creator.createCount != 0 {
		t.Fatalf("invalid generated draft must not be persisted, creates=%d", creator.createCount)
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("LLM calls = %d, want exactly two bounded attempts", fakeLLM.calls)
	}
}

func TestDetectionPackageGenerationServiceCorrectsForbiddenBPFHelperBeforePersist(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{
		detectionPackageGenerationResponseWithSource(
			"syscalls/sys_enter_execve",
			"int detector(void *ctx) { bpf_probe_read_kernel(0, 0, 0); return 0; }",
		),
		detectionPackageGenerationResponse("syscalls/sys_enter_execve"),
	}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	_, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "AF_ALG exploit chain",
	})
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("LLM calls = %d, want one generation plus one correction", fakeLLM.calls)
	}
	if !strings.Contains(fakeLLM.prompts[0], "bpf_probe_read_kernel") {
		t.Fatalf("initial prompt did not include builder helper policy: %q", fakeLLM.prompts[0])
	}
	if !strings.Contains(fakeLLM.prompts[1], "forbidden BPF helper call: bpf_probe_read_kernel") {
		t.Fatalf("correction prompt did not include helper rejection: %q", fakeLLM.prompts[1])
	}
	if strings.Contains(creator.request.EBPFSource, "bpf_probe_read_kernel") {
		t.Fatalf("forbidden helper reached persisted draft: %q", creator.request.EBPFSource)
	}
}

func TestDetectionPackageGenerationServiceCorrectsUnsupportedKernelAnnotationBeforePersist(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{
		detectionPackageGenerationResponseWithSource(
			"syscalls/sys_enter_execve",
			"static int read_path(const void __user *src) { return bpf_probe_read_user(0, 0, src); }",
		),
		detectionPackageGenerationResponse("syscalls/sys_enter_execve"),
	}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	_, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "AF_ALG exploit chain",
	})
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("LLM calls = %d, want one generation plus one correction", fakeLLM.calls)
	}
	if !strings.Contains(fakeLLM.prompts[1], "unsupported kernel source annotation: __user") {
		t.Fatalf("correction prompt did not contain the annotation rejection: %q", fakeLLM.prompts[1])
	}
	if strings.Contains(creator.request.EBPFSource, "__user") {
		t.Fatalf("unsupported kernel annotation reached persisted draft: %q", creator.request.EBPFSource)
	}
}

func TestDetectionPackageGenerationServiceRejectsUnsupportedCoverageWithoutPersist(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	unsupported := `
## Coverage Decision
` + "```yaml" + `
status: unsupported
reason: The active allowlist cannot observe AF_ALG, pipe, or splice.
covered_behaviors: []
uncovered_core_behaviors: [AF_ALG, pipe, splice]
required_hooks:
  - attach_type: tracepoint
    attach: syscalls/sys_enter_socket
  - attach_type: tracepoint
    attach: syscalls/sys_enter_bind
  - attach_type: tracepoint
    attach: syscalls/sys_enter_splice
` + "```" + `
## Risks and Limitations
The requested exploit chain is not observable.
`
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{unsupported, unsupported}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	_, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "AF_ALG exploit chain",
	})
	if err == nil || !strings.Contains(err.Error(), "active hook allowlist cannot faithfully observe") {
		t.Fatalf("expected unsupported coverage failure, got %v", err)
	}
	if creator.createCount != 0 {
		t.Fatalf("unsupported package must not be persisted, creates=%d", creator.createCount)
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("unsupported coverage must perform one bounded completeness confirmation, calls=%d", fakeLLM.calls)
	}
	var described interface {
		RecoveryDescriptor() recovery.Descriptor
	}
	if !errors.As(err, &described) {
		t.Fatalf("unsupported coverage error did not preserve recovery contract: %T", err)
	}
	descriptor := described.RecoveryDescriptor()
	if descriptor.Code != "detection_package_hook_coverage_blocked" {
		t.Fatalf("unexpected recovery code: %q", descriptor.Code)
	}
	if len(descriptor.Actions) == 0 || descriptor.Actions[0].ID != "extend_hook_allowlist" {
		t.Fatalf("validated required hooks did not expose allowlist recovery: %#v", descriptor.Actions)
	}
}

func TestDetectionPackageGenerationServiceAggregatesUnsupportedCoverageBeforeRecovery(t *testing.T) {
	creator := &fakeDetectionPackageDraftCreator{allowlist: &AllowlistConfig{
		Tracepoints: []string{"syscalls/sys_enter_execve"},
	}}
	fakeLLM := &fakeDetectionPackageLLM{responses: []string{
		unsupportedCoverageResponse(
			"AF_ALG socket is not observable.",
			"AF_ALG socket creation",
			"syscalls/sys_enter_socket",
		),
		unsupportedCoverageResponse(
			"File opening is also not observable.",
			"opening the target file",
			"syscalls/sys_enter_openat",
		),
	}}
	svc := NewDetectionPackageGenerationService(fakeDetectionPackageGenerationConfigRepo{}, creator, 30, 1)
	svc.newLLMClient = func(string, string, string, int, int) detectionPackageLLMCaller {
		return fakeLLM
	}

	_, err := svc.GenerateDraft(context.Background(), GenerateDetectionPackageDraftRequest{
		CVEID:                    "CVE-2026-31431",
		VulnerabilityDescription: "open target -> AF_ALG socket -> splice",
	})
	if err == nil {
		t.Fatal("expected unsupported coverage recovery")
	}
	if fakeLLM.calls != 2 {
		t.Fatalf("LLM calls = %d, want a bounded completeness confirmation", fakeLLM.calls)
	}
	var described interface {
		RecoveryDescriptor() recovery.Descriptor
	}
	if !errors.As(err, &described) {
		t.Fatalf("unsupported coverage error did not preserve recovery contract: %T", err)
	}
	contextData := described.RecoveryDescriptor().Context
	required, ok := contextData["required_hooks"].([]detectionPackageRequiredHook)
	if !ok {
		t.Fatalf("required hook context has unexpected type: %#v", contextData["required_hooks"])
	}
	if len(required) != 2 {
		t.Fatalf("required hooks were not aggregated across coverage passes: %#v", required)
	}
	if !strings.Contains(fakeLLM.prompts[1], "syscalls/sys_enter_socket") ||
		!strings.Contains(fakeLLM.prompts[1], "complete exploit chain") {
		t.Fatalf("coverage confirmation prompt lacks prior evidence: %q", fakeLLM.prompts[1])
	}
}

func unsupportedCoverageResponse(reason, behavior, hook string) string {
	return `
## Coverage Decision
` + "```yaml" + `
status: unsupported
reason: ` + reason + `
covered_behaviors: []
uncovered_core_behaviors:
  - ` + behavior + `
required_hooks:
  - attach_type: tracepoint
    attach: ` + hook + `
` + "```" + `
## Risks and Limitations
The requested exploit chain is not observable.
`
}

func detectionPackageGenerationResponse(attach string) string {
	return detectionPackageGenerationResponseWithSource(attach, "int detector(void *ctx) { return 0; }")
}

func detectionPackageGenerationResponseWithSource(attach, source string) string {
	return `
## Coverage Decision
` + "```yaml" + `
status: supported
reason: The active hook observes the requested behavior.
covered_behaviors: [requested_behavior]
uncovered_core_behaviors: []
` + "```" + `
## HookPlan
` + "```yaml" + `
schema_version: "aegis.ebpf_plugin.v1"
package_id: "test-package"
version: "1.0.0"
hooks:
  - name: generated_hook
    attach_type: tracepoint
    attach: ` + attach + `
    program: tracepoint__generated_hook
` + "```" + `
## eBPF Source
` + "```c" + `
` + source + `
` + "```" + `
## Sigma
` + "```yaml" + `
title: CVE runtime detector
` + "```" + `
## Correlation
` + "```yaml" + `
sequence: [event]
` + "```" + `
`
}
