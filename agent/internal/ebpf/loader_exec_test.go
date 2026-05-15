package ebpf

import (
	"errors"
	"testing"
)

func TestArgvBytesToCommandLineDecodesNULSeparatedArgv(t *testing.T) {
	raw := make([]byte, 512)
	copy(raw, []byte("nc\x00-lvnp\x001234\x00"))

	got := argvBytesToCommandLine(raw)
	want := "nc -lvnp 1234"
	if got != want {
		t.Fatalf("argvBytesToCommandLine() = %q, want %q", got, want)
	}
}

func TestBuildExecEventPrefersEBPFArgvOverStaleProcCmdline(t *testing.T) {
	loader := &Loader{
		hostID:   "host-1",
		hostname: "test-host",
	}

	var exec ExecEvent
	exec.Pid = 1788923
	exec.Uid = 0
	copy(exec.Comm[:], "bash")
	copy(exec.Filename[:], "/usr/bin/nc")
	copy(exec.Args[:], []byte("nc\x00-lvnp\x001234\x00"))

	event := loader.buildExecRuntimeEvent(exec, execRuntimeReaders{
		readCmdline: func(pid uint32) (string, error) {
			if pid != exec.Pid {
				t.Fatalf("readCmdline pid = %d, want %d", pid, exec.Pid)
			}
			return "-bash", nil
		},
		readExe: func(pid uint32) (string, error) {
			return "", errors.New("not needed")
		},
	})

	if event.CommandLine != "nc -lvnp 1234" {
		t.Fatalf("CommandLine = %q, want %q", event.CommandLine, "nc -lvnp 1234")
	}
	if event.FilePath != "/usr/bin/nc" {
		t.Fatalf("FilePath = %q, want %q", event.FilePath, "/usr/bin/nc")
	}
	if event.ProcessName != "nc" {
		t.Fatalf("ProcessName = %q, want %q", event.ProcessName, "nc")
	}
}

func TestBuildExecEventUsesProcCmdlineOnlyWhenEBPFDataMissing(t *testing.T) {
	loader := &Loader{
		hostID:   "host-1",
		hostname: "test-host",
	}

	var exec ExecEvent
	exec.Pid = 42
	copy(exec.Comm[:], "unknown")

	event := loader.buildExecRuntimeEvent(exec, execRuntimeReaders{
		readCmdline: func(pid uint32) (string, error) {
			return "/usr/bin/id -u", nil
		},
		readExe: func(pid uint32) (string, error) {
			return "/usr/bin/id", nil
		},
	})

	if event.CommandLine != "/usr/bin/id -u" {
		t.Fatalf("CommandLine = %q, want %q", event.CommandLine, "/usr/bin/id -u")
	}
	if event.FilePath != "/usr/bin/id" {
		t.Fatalf("FilePath = %q, want %q", event.FilePath, "/usr/bin/id")
	}
	if event.ProcessName != "id" {
		t.Fatalf("ProcessName = %q, want %q", event.ProcessName, "id")
	}
}

func TestBuildExecEventPreservesArgvTruncationFlag(t *testing.T) {
	loader := &Loader{
		hostID:   "host-1",
		hostname: "test-host",
	}

	var exec ExecEvent
	exec.Pid = 1001
	exec.Flags = ExecEventArgsTruncated
	copy(exec.Filename[:], "/usr/bin/nc")
	copy(exec.Args[:], []byte("nc\x00-lvnp\x001234\x00"))

	event := loader.buildExecRuntimeEvent(exec, execRuntimeReaders{})
	if !event.ArgsTruncated {
		t.Fatal("ArgsTruncated = false, want true")
	}
}
