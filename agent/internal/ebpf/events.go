package ebpf

const ExecEventArgsTruncated uint32 = 1 << 0

type ExecEvent struct {
	Pid      uint32
	Ppid     uint32
	Uid      uint32
	Gid      uint32
	Flags    uint32
	Comm     [16]byte
	Filename [256]byte
	Args     [512]byte
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
