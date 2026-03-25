package ebpf

type ExecEvent struct {
	Pid      uint32
	Ppid     uint32
	Uid      uint32
	Gid      uint32
	Comm     [16]byte
	Filename [256]byte
	Args     [256]byte
}

type ForkEvent struct {
	ParentPid  uint32
	ChildPid   uint32
	Uid        uint32
	ParentComm [16]byte
	ChildComm  [16]byte
}

func bytesToString(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}
