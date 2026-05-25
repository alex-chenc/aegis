package correlation

import (
	"strconv"
)

type ProcessContext struct {
	PID         int
	PPID        int
	UID         int
	GID         int
	Comm        string
	Name        string
	CommandLine string
	ExePath     string
	TreeKey     string
}

func ComputeTreeKey(hostID string, procCtx ProcessContext) string {
	// TODO: Read /proc/{pid}/status PPid chain to find nearest stable ancestor (session leader or init)
	return hostID + "_" + strconv.Itoa(procCtx.PID)
}
