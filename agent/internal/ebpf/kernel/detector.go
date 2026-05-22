package kernel

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/features"
	"golang.org/x/sys/unix"
)

// EventTransport represents the eBPF event transport mechanism.
type EventTransport string

const (
	TransportDisabled EventTransport = "disabled"
	TransportRingbuf  EventTransport = "ringbuf"
	TransportPerf     EventTransport = "perf"
)

// Capabilities describes the kernel's eBPF capabilities.
type Capabilities struct {
	KernelRelease    string
	Major            int
	Minor            int
	Patch            int
	BTFAvailable     bool
	RingbufAvailable bool
	Transport        EventTransport
	DisabledReason   string
}

// Detect probes the running kernel and returns eBPF capabilities.
func Detect() *Capabilities {
	caps := &Capabilities{}
	caps.KernelRelease = detectKernelRelease()
	caps.Major, caps.Minor, caps.Patch = parseKernelVersion(caps.KernelRelease)
	caps.BTFAvailable = haveKernelBTF()
	caps.RingbufAvailable = features.HaveMapType(ebpf.RingBuf) == nil
	caps.Transport, caps.DisabledReason = selectTransport(caps)
	return caps
}

func haveKernelBTF() bool {
	spec, err := btf.LoadKernelSpec()
	return err == nil && spec != nil
}

func detectKernelRelease() string {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err == nil {
		n := 0
		for n < len(uname.Release) && uname.Release[n] != 0 {
			n++
		}
		return string(uname.Release[:n])
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return "0.0.0"
	}
	return strings.TrimSpace(string(data))
}

func parseKernelVersion(release string) (major, minor, patch int) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return 0, 0, 0
	}
	major, _ = strconv.Atoi(parts[0])
	minor, _ = strconv.Atoi(parts[1])
	if len(parts) >= 3 {
		patchStr := parts[2]
		for i, c := range patchStr {
			if c < '0' || c > '9' {
				patchStr = patchStr[:i]
				break
			}
		}
		patch, _ = strconv.Atoi(patchStr)
	}
	return major, minor, patch
}

func selectTransport(caps *Capabilities) (EventTransport, string) {
	if caps.Major < 4 || (caps.Major == 4 && caps.Minor < 18) {
		return TransportDisabled, fmt.Sprintf("kernel %d.%d below 4.18", caps.Major, caps.Minor)
	}
	if !caps.BTFAvailable {
		return TransportDisabled, "BTF/CO-RE unavailable"
	}
	if caps.RingbufAvailable && (caps.Major > 5 || (caps.Major == 5 && caps.Minor >= 8)) {
		return TransportRingbuf, ""
	}
	return TransportPerf, ""
}
