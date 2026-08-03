package ebpf

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestLSMPolicyPathKeyABIHasNoPaddingBeforePrefixData(t *testing.T) {
	key, err := buildLSMPolicyPathKey(0x0102030405060708, "/run/containerd/containerd.sock")
	if err != nil {
		t.Fatal(err)
	}
	if unsafe.Sizeof(key) != 268 {
		t.Fatalf("policy path key ABI size=%d want=268", unsafe.Sizeof(key))
	}
	if binary.LittleEndian.Uint64(key.Data[:8]) != 0x0102030405060708 ||
		string(key.Data[8:8+len("/run/containerd/containerd.sock")]) != "/run/containerd/containerd.sock" {
		t.Fatalf("policy slot/path do not start at LPM data byte zero: %#v", key)
	}
	if key.PrefixLen != 64+uint32(len("/run/containerd/containerd.sock"))*8 {
		t.Fatalf("prefixlen=%d", key.PrefixLen)
	}
}
