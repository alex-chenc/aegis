package ebpf

import (
	"errors"
	"testing"
)

func TestLoadConfiguredProgramsReturnsExecveLoadError(t *testing.T) {
	loader := &Loader{}
	execErr := errors.New("verifier rejected execve")

	err := loader.loadConfiguredPrograms(defaultBPFPrograms(), func(name, tracepoint, category, mapName string) error {
		if name == "execve" {
			return execErr
		}
		return nil
	})

	if !errors.Is(err, execErr) {
		t.Fatalf("loadConfiguredPrograms error = %v, want %v", err, execErr)
	}
}

func TestLoadConfiguredProgramsAllowsOptionalForkLoadError(t *testing.T) {
	loader := &Loader{}
	forkErr := errors.New("fork unavailable")

	err := loader.loadConfiguredPrograms(defaultBPFPrograms(), func(name, tracepoint, category, mapName string) error {
		if name == "fork" {
			return forkErr
		}
		return nil
	})

	if err != nil {
		t.Fatalf("loadConfiguredPrograms error = %v, want nil", err)
	}
}
