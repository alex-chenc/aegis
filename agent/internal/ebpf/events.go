package ebpf

// ExecEvent matches execve.bpf.c struct exec_event
type ExecEvent struct {
	Pid      uint32
	Ppid     uint32
	Uid      uint32
	Gid      uint32
	Comm     [16]byte
	Filename [256]byte
	Args     [256]byte
}

// ForkEvent matches fork.bpf.c struct fork_event
type ForkEvent struct {
	ParentPid  uint32
	ChildPid   uint32
	Uid        uint32
	ParentComm [16]byte
	ChildComm  [16]byte
}

// ExitEvent matches exit.bpf.c struct exit_event
type ExitEvent struct {
	Pid      uint32
	Uid      uint32
	ExitCode int32
	Comm     [16]byte
}

// FileEvent matches openat.bpf.c struct file_event
type FileEvent struct {
	Pid      uint32
	Uid      uint32
	Flags    int32
	Comm     [16]byte
	Filename [256]byte
}

// ConnEvent matches connect.bpf.c struct conn_event
type ConnEvent struct {
	Pid   uint32
	Uid   uint32
	Comm  [16]byte
	Saddr [4]byte
	Daddr [4]byte
	Sport uint16
	Dport uint16
}

// PrivEvent matches setuid.bpf.c/setgid.bpf.c struct priv_event
type PrivEvent struct {
	Pid       uint32
	Uid       uint32
	TargetUID uint32
	Syscall   [16]byte
	Comm      [16]byte
}

// CapEvent matches capset.bpf.c struct cap_event
type CapEvent struct {
	Pid          uint32
	Uid          uint32
	CapEffective uint64
	CapPermitted uint64
	Syscall      [16]byte
	Comm         [16]byte
}

// bytesToString converts a byte array to a string, stopping at null terminator
func bytesToString(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}
