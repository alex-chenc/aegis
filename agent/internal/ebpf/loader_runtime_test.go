//go:build ebpf_runtime

package ebpf

import (
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRuntimeLoaderReceivesProcessFileAndNetworkEvents(t *testing.T) {
	events := make(chan Event, 1024)
	loader, err := NewLoader("runtime-host", events)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	if err := loader.LoadAll(); err != nil {
		loader.Close()
		t.Fatalf("LoadAll: %v", err)
	}
	defer loader.Close()

	if _, ok := loader.readers["execve"]; !ok {
		t.Fatal("execve reader not loaded")
	}
	if _, ok := loader.readers["file"]; !ok {
		t.Fatal("file reader not loaded")
	}
	if _, ok := loader.readers["tcp_connect"]; !ok {
		t.Fatal("tcp_connect reader not loaded")
	}

	if err := exec.Command("/bin/true").Run(); err != nil {
		t.Fatalf("trigger exec: %v", err)
	}

	tmp := "/tmp/aegis-ebpf-runtime-test"
	if err := os.WriteFile(tmp, []byte("runtime-test"), 0600); err != nil {
		t.Fatalf("trigger file write: %v", err)
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		t.Fatalf("trigger chmod: %v", err)
	}
	if err := exec.Command("/bin/sh", "-c", "echo runtime-shell > /tmp/aegis-ebpf-runtime-test").Run(); err != nil {
		t.Fatalf("trigger shell file write: %v", err)
	}
	_ = os.Remove(tmp)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	expectedRemote := ln.Addr().String()
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
		close(accepted)
	}()
	conn, err := net.DialTimeout("tcp", expectedRemote, time.Second)
	if err != nil {
		t.Fatalf("trigger tcp connect: %v", err)
	}
	_ = conn.Close()
	<-accepted

	deadline := time.After(5 * time.Second)
	seen := map[string]bool{}
	for {
		if seen["process_exec"] && seen["file_access"] && seen["network_connect"] {
			return
		}
		select {
		case event := <-events:
			t.Logf("event type=%s path=%q remote=%q cmd=%q", event.EventType, event.FilePath, event.RemoteAddr, event.CommandLine)
			if event.EventType == "file_access" && event.FilePath != tmp {
				continue
			}
			if event.EventType == "network_connect" && event.RemoteAddr != expectedRemote {
				continue
			}
			seen[event.EventType] = true
		case <-deadline:
			t.Fatalf("timed out waiting for events, seen=%v", seen)
		}
	}
}
