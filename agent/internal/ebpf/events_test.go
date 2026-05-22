package ebpf

import (
	"encoding/binary"
	"testing"
)

func TestParseFileEventMatchesBPFLayout(t *testing.T) {
	raw := make([]byte, 564)
	binary.LittleEndian.PutUint32(raw[8:12], 1234)
	binary.LittleEndian.PutUint32(raw[16:20], 1000)
	binary.LittleEndian.PutUint32(raw[24:28], 0x0040|0x0200)
	binary.LittleEndian.PutUint32(raw[32:36], FileActionTruncate)
	copy(raw[36:52], []byte("bash"))
	copy(raw[52:308], []byte("/etc/passwd"))
	copy(raw[308:564], []byte("/etc/passwd.old"))

	event, err := parseFileEvent(raw)
	if err != nil {
		t.Fatalf("parseFileEvent returned error: %v", err)
	}
	if event.Pid != 1234 {
		t.Fatalf("Pid = %d, want 1234", event.Pid)
	}
	if got := bytesToString(event.Path[:]); got != "/etc/passwd" {
		t.Fatalf("Path = %q, want /etc/passwd", got)
	}
	if got := bytesToString(event.OldPath[:]); got != "/etc/passwd.old" {
		t.Fatalf("OldPath = %q, want /etc/passwd.old", got)
	}
}

func TestParseFlagsToStringsUsesLinuxOpenFlagValues(t *testing.T) {
	flags := parseFlagsToStrings(0x0001 | 0x0040 | 0x0200 | 0x0400)
	want := map[string]bool{
		"O_WRONLY": true,
		"O_CREAT":  true,
		"O_TRUNC":  true,
		"O_APPEND": true,
	}
	for _, flag := range flags {
		delete(want, flag)
	}
	if len(want) != 0 {
		t.Fatalf("missing flags: %v; got %v", want, flags)
	}
}
