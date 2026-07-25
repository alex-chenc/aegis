package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/recovery"
)

func TestPackageBuildToolsDeclareAsynchronousCompletionContract(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterPackageWriteTools(registry, PackageWriteToolDeps{}); err != nil {
		t.Fatalf("register package write tools: %v", err)
	}

	start, ok := registry.Get("Package.Build.Start")
	if !ok {
		t.Fatal("Package.Build.Start was not registered")
	}
	if start.ExecutionContract.Mode != assistant.ToolExecutionAsynchronous {
		t.Fatalf("build execution mode = %q, want asynchronous", start.ExecutionContract.Mode)
	}
	if start.ExecutionContract.CompletionCapability != "get_detection_package_build_status" {
		t.Fatalf("build completion capability = %q", start.ExecutionContract.CompletionCapability)
	}

	status, ok := registry.Get("Package.Build.Status")
	if !ok {
		t.Fatal("Package.Build.Status was not registered")
	}
	if status.Capability != start.ExecutionContract.CompletionCapability {
		t.Fatalf("status capability %q does not close build capability %q", status.Capability, start.ExecutionContract.CompletionCapability)
	}
	if len(status.ResultContract.PendingValues) == 0 || len(status.ResultContract.SuccessValues) == 0 || len(status.ResultContract.FailureValues) == 0 {
		t.Fatalf("build status terminal contract is incomplete: %#v", status.ResultContract)
	}
}

func TestPackageDraftGenerateDeclaresConfiguredLongRunningTimeout(t *testing.T) {
	registry := assistant.NewToolRegistry()
	configuredTimeout := 20 * time.Minute
	if err := RegisterPackageWriteTools(registry, PackageWriteToolDeps{
		DraftGenerationTimeout: configuredTimeout,
	}); err != nil {
		t.Fatalf("register package write tools: %v", err)
	}

	draft, ok := registry.Get("Package.Draft.Generate")
	if !ok {
		t.Fatal("Package.Draft.Generate was not registered")
	}
	if draft.DefaultTimeout != configuredTimeout {
		t.Fatalf("draft timeout = %s, want %s", draft.DefaultTimeout, configuredTimeout)
	}
	if !strings.Contains(strings.ToLower(draft.Description), "ai") ||
		!strings.Contains(strings.ToLower(draft.Description), "vulnerability") {
		t.Fatalf("draft description must state that AI generates artifacts from vulnerability input: %q", draft.Description)
	}
}

type failedBuildPackageService struct {
	DetectionPackageServiceForTools
}

func (failedBuildPackageService) GetLatestBuild(context.Context, string) (*model.DetectionPackageBuild, error) {
	return &model.DetectionPackageBuild{
		PackageID:    "package-1",
		Status:       "failed",
		ErrorMessage: "compile perf: exit status 1",
		BuildLog:     "plugin.c:179:23: error: expected ')'",
	}, nil
}

func TestPackageBuildStatusReturnsRecoverableBuildFailure(t *testing.T) {
	_, err := makePackageBuildStatusHandler(failedBuildPackageService{})(context.Background(), map[string]interface{}{
		"package_id": "package-1",
	})
	if err == nil {
		t.Fatal("failed build did not become a recoverable blocker")
	}
	var described interface {
		RecoveryDescriptor() recovery.Descriptor
	}
	if !errors.As(err, &described) {
		t.Fatalf("build failure does not implement recovery contract: %T %v", err, err)
	}
	descriptor := described.RecoveryDescriptor()
	if descriptor.Code != "detection_package_build_failed" {
		t.Fatalf("unexpected recovery code: %q", descriptor.Code)
	}
	if len(descriptor.Actions) == 0 || descriptor.Actions[0].ID != "regenerate_detection_package" ||
		!descriptor.Actions[0].ResumesRun {
		t.Fatalf("build failure does not expose regenerate-and-resume: %#v", descriptor.Actions)
	}
}
