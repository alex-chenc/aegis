package ebpf

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"aegis-agent/internal/ebpf/kernel"
	"aegis-agent/internal/logger"
	"aegis-agent/internal/tools"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"
)

// Loader manages eBPF programs and event readers
type Loader struct {
	collections  map[string]*ebpf.Collection
	links        []link.Link
	readers      map[string]EventReader
	eventChan    chan Event
	hostID       string
	hostname     string
	seq          uint64
	done         chan struct{}
	dropCount    uint64
	capabilities *kernel.Capabilities
	toolManager  *tools.ToolManager
}

type execRuntimeReaders struct {
	readCmdline func(pid uint32) (string, error)
	readExe     func(pid uint32) (string, error)
}

type bpfProgramConfig struct {
	name       string
	attachType string // "tracepoint" or "kprobe"
	category   string
	symbol     string
	progName   string
	mapName    string
	required   bool
}

// NewLoader creates a new eBPF loader with kernel capability detection.
func NewLoader(hostID string, eventChan chan Event) (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("remove memlock limit: %w", err)
	}

	caps := kernel.Detect()
	logger.Info("[eBPF] capability detected",
		zap.String("kernel", caps.KernelRelease),
		zap.Bool("btf", caps.BTFAvailable),
		zap.String("transport", string(caps.Transport)))

	if caps.Transport == kernel.TransportDisabled {
		return nil, fmt.Errorf("eBPF event engine disabled: %s", caps.DisabledReason)
	}

	hostname, _ := os.Hostname()
	return &Loader{
		collections:  make(map[string]*ebpf.Collection),
		readers:      make(map[string]EventReader),
		eventChan:    eventChan,
		hostID:       hostID,
		hostname:     hostname,
		done:         make(chan struct{}),
		capabilities: caps,
		toolManager:  tools.NewToolManager(),
	}, nil
}

// LoadAll loads eBPF programs for all event types.
func (l *Loader) LoadAll() error {
	return l.loadConfiguredPrograms(defaultBPFPrograms())
}

func defaultBPFPrograms() []bpfProgramConfig {
	return []bpfProgramConfig{
		{name: "execve", attachType: "tracepoint", category: "syscalls", symbol: "sys_enter_execve", mapName: "exec_events", required: true},
		{name: "fork", attachType: "tracepoint", category: "sched", symbol: "sched_process_fork", mapName: "fork_events"},
		{name: "file", attachType: "tracepoint", category: "syscalls", symbol: "sys_enter_openat", progName: "trace_openat", mapName: "file_events"},
		{name: "tcp_connect", attachType: "kprobe", mapName: "conn_events"},
		{name: "accept", attachType: "tracepoint", category: "sock", symbol: "inet_sock_set_state", progName: "trace_inet_sock_set_state", mapName: "accept_events"},
	}
}

func (l *Loader) loadConfiguredPrograms(programs []bpfProgramConfig) error {
	for _, prog := range programs {
		if err := l.loadProgram(prog); err != nil {
			logger.Warn("Failed to load eBPF program",
				zap.String("program", prog.name),
				zap.Error(err))
			if prog.required {
				return fmt.Errorf("required eBPF program %s failed to load: %w", prog.name, err)
			}
			continue
		}
	}
	return nil
}

func (l *Loader) loadProgram(cfg bpfProgramConfig) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	suffix := BPFObjectSuffix(l.capabilities)

	objPaths := []string{
		filepath.Join(execDir, "bpf", cfg.name+suffix),
		filepath.Join(execDir, "..", "internal", "ebpf", "bpf", "obj", cfg.name+suffix),
		filepath.Join("internal/ebpf/bpf/obj", cfg.name+suffix),
		filepath.Join("bpf/obj", cfg.name+suffix),
	}

	var spec *ebpf.CollectionSpec
	for _, objPath := range objPaths {
		spec, err = ebpf.LoadCollectionSpec(objPath)
		if err == nil {
			break
		}
	}
	if spec == nil {
		return fmt.Errorf("failed to load spec for %s: tried paths %v, last error: %w", cfg.name, objPaths, err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to load collection for %s: %w", cfg.name, err)
	}

	// Attach main program
	var localLinks []link.Link
	if cfg.name == "tcp_connect" {
		localLinks, err = attachTCPConnect(coll)
	} else {
		var ln link.Link
		progName := cfg.progName
		if progName == "" {
			progName = "trace_" + cfg.name
		}
		prog, ok := coll.Programs[progName]
		if !ok {
			coll.Close()
			return fmt.Errorf("program %s not found in collection for %s", progName, cfg.name)
		}
		ln, err = link.Tracepoint(cfg.category, cfg.symbol, prog, nil)
		if err == nil {
			localLinks = append(localLinks, ln)
		}
	}
	if err != nil {
		for _, ln := range localLinks {
			_ = ln.Close()
		}
		coll.Close()
		return fmt.Errorf("failed to attach %s for %s: %w", cfg.attachType, cfg.name, err)
	}

	// Attach additional file tracepoints
	if cfg.name == "file" {
		localLinks = append(localLinks, attachExtraFileTracepoints(coll)...)
	}

	// Create event reader
	eventMap, ok := coll.Maps[cfg.mapName]
	if !ok {
		coll.Close()
		return fmt.Errorf("map %s not found in collection for %s", cfg.mapName, cfg.name)
	}

	rd, err := NewEventReader(eventMap, l.capabilities)
	if err != nil {
		for _, ln := range localLinks {
			_ = ln.Close()
		}
		coll.Close()
		return fmt.Errorf("failed to create event reader for %s: %w", cfg.name, err)
	}
	l.collections[cfg.name] = coll
	l.links = append(l.links, localLinks...)
	l.readers[cfg.name] = rd

	go l.readEvents(cfg.name, rd)

	logger.Info("eBPF program loaded",
		zap.String("program", cfg.name),
		zap.String("attach", cfg.attachType),
		zap.String("transport", string(l.capabilities.Transport)))

	return nil
}

func attachTCPConnect(coll *ebpf.Collection) ([]link.Link, error) {
	type probe struct {
		symbol string
		entry  string
		ret    string
	}
	probes := []probe{
		{symbol: "tcp_v4_connect", entry: "kprobe_tcp_v4_connect", ret: "kretprobe_tcp_v4_connect"},
		{symbol: "tcp_v6_connect", entry: "kprobe_tcp_v6_connect", ret: "kretprobe_tcp_v6_connect"},
	}

	var links []link.Link
	for _, p := range probes {
		entryProg, entryOK := coll.Programs[p.entry]
		retProg, retOK := coll.Programs[p.ret]
		if !entryOK || !retOK {
			for _, ln := range links {
				_ = ln.Close()
			}
			return nil, fmt.Errorf("tcp connect programs missing for %s", p.symbol)
		}
		entryLink, err := link.Kprobe(p.symbol, entryProg, nil)
		if err != nil {
			if p.symbol == "tcp_v6_connect" {
				logger.Warn("Failed to attach tcp_v6_connect kprobe", zap.Error(err))
				continue
			}
			for _, ln := range links {
				_ = ln.Close()
			}
			return nil, fmt.Errorf("attach kprobe %s: %w", p.symbol, err)
		}
		retLink, err := link.Kretprobe(p.symbol, retProg, nil)
		if err != nil {
			_ = entryLink.Close()
			if p.symbol == "tcp_v6_connect" {
				logger.Warn("Failed to attach tcp_v6_connect kretprobe", zap.Error(err))
				continue
			}
			for _, ln := range links {
				_ = ln.Close()
			}
			return nil, fmt.Errorf("attach kretprobe %s: %w", p.symbol, err)
		}
		links = append(links, entryLink, retLink)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no tcp_connect probes attached")
	}
	return links, nil
}

func attachExtraFileTracepoints(coll *ebpf.Collection) []link.Link {
	extraTPs := []struct {
		progName string
		symbol   string
	}{
		{"trace_openat2", "sys_enter_openat2"},
		{"trace_creat", "sys_enter_creat"},
		{"trace_unlinkat", "sys_enter_unlinkat"},
		{"trace_renameat", "sys_enter_renameat"},
		{"trace_renameat2", "sys_enter_renameat2"},
		{"trace_chmod", "sys_enter_chmod"},
		{"trace_fchmodat", "sys_enter_fchmodat"},
		{"trace_chown", "sys_enter_chown"},
		{"trace_fchownat", "sys_enter_fchownat"},
	}
	var links []link.Link
	for _, tp := range extraTPs {
		prog, ok := coll.Programs[tp.progName]
		if !ok {
			continue
		}
		ln, err := link.Tracepoint("syscalls", tp.symbol, prog, nil)
		if err != nil {
			logger.Debug("Extra tracepoint not attached",
				zap.String("symbol", tp.symbol),
				zap.Error(err))
			continue
		}
		links = append(links, ln)
	}
	return links
}

func (l *Loader) readEvents(name string, rd EventReader) {
	logger.Info("Event reader started", zap.String("program", name))
	for {
		select {
		case <-l.done:
			return
		default:
		}
		data, err := rd.Read()
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				return
			}
			logger.Debug("Event read error", zap.String("program", name), zap.Error(err))
			continue
		}
		l.processEvent(name, data)
	}
}

func (l *Loader) processEvent(name string, data []byte) {
	switch name {
	case "execve":
		l.processExecEvent(data)
	case "fork":
		l.processForkEvent(data)
	case "file":
		l.processFileEvent(data)
	case "tcp_connect":
		l.processConnEvent(data)
	case "accept":
		l.processAcceptEvent(data)
	}
}

func (l *Loader) processExecEvent(data []byte) {
	var e ExecEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}
	l.sendEvent(l.buildExecRuntimeEvent(e, execRuntimeReaders{
		readCmdline: readProcCmdline,
		readExe:     readProcExe,
	}))
}

func (l *Loader) buildExecRuntimeEvent(e ExecEvent, readers execRuntimeReaders) Event {
	filename := bytesToString(e.Filename[:])
	argvCmd := argvBytesToCommandLine(e.Args[:])
	comm := strings.TrimSpace(bytesToString(e.Comm[:]))
	eBPFFilename := filename

	if filename == "" && readers.readExe != nil {
		if exePath, err := readers.readExe(e.Pid); err == nil {
			filename = strings.TrimSpace(exePath)
		}
	}

	cmdLine := argvCmd
	if cmdLine == "" && eBPFFilename != "" {
		cmdLine = eBPFFilename
	}
	if cmdLine == "" && eBPFFilename == "" && readers.readCmdline != nil {
		if procCmdline, err := readers.readCmdline(e.Pid); err == nil {
			cmdLine = normalizeCommandLine(procCmdline)
		}
	}

	return Event{
		EventID:       l.nextEventID(),
		HostID:        l.hostID,
		Hostname:      l.hostname,
		Timestamp:     time.Now().UnixMilli(),
		EventType:     "process_exec",
		ProcessName:   processNameFromExec(comm, filename, cmdLine),
		PID:           int(e.Pid),
		PPID:          int(e.Ppid),
		UID:           int(e.Uid),
		CommandLine:   cmdLine,
		FilePath:      filename,
		ArgsTruncated: e.Flags&ExecEventArgsTruncated != 0,
	}
}

func (l *Loader) processForkEvent(data []byte) {
	var e ForkEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}
	l.sendEvent(Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "process_fork",
		ProcessName: bytesToString(e.ParentComm[:]),
		PID:         int(e.ChildPid),
		PPID:        int(e.ParentPid),
		UID:         int(e.Uid),
		CommandLine: fmt.Sprintf("fork from %s", bytesToString(e.ParentComm[:])),
	})
}

func (l *Loader) processFileEvent(data []byte) {
	e, err := parseFileEvent(data)
	if err != nil {
		logger.Debug("Failed to parse file event", zap.Error(err))
		return
	}

	path := bytesToString(e.Path[:])
	oldPath := bytesToString(e.OldPath[:])
	comm := strings.TrimSpace(bytesToString(e.Comm[:]))
	action := fileActionName(e.Action)
	flags := parseFlagsToStrings(e.Flags)

	logger.Info("[eBPF] File event captured",
		zap.String("path", path),
		zap.String("action", action),
		zap.Uint32("raw_action", e.Action),
		zap.Int32("raw_flags", e.Flags),
		zap.String("comm", comm),
		zap.Uint32("pid", e.Pid))

	cmdLine := ""
	if exePath, err := readProcExe(e.Pid); err == nil {
		cmdLine = strings.TrimSpace(exePath)
	}

	// Capture process tree at event time (before short-lived process exits)
	processTreeJSON := l.captureProcessTreeJSON(int(e.Pid))

	dir, name := filePathParts(path)

	l.sendEvent(Event{
		EventID:        l.nextEventID(),
		HostID:         l.hostID,
		Hostname:       l.hostname,
		Timestamp:      time.Now().UnixMilli(),
		EventType:      "file_access",
		ProcessName:    comm,
		PID:            int(e.Pid),
		UID:            int(e.Uid),
		CommandLine:    cmdLine,
		FilePath:       path,
		FileName:       name,
		FileDir:        dir,
		FileAction:     action,
		FileFlags:      strings.Join(flags, ","),
		OldFilePath:    oldPath,
		ProcessTreeJSON: processTreeJSON,
	})
}

func (l *Loader) processConnEvent(data []byte) {
	e, err := parseConnEvent(data)
	if err != nil {
		logger.Debug("Failed to parse conn event", zap.Error(err))
		return
	}

	srcIP, dstIP, srcPort, dstPort := parseConnAddr(e)
	comm := strings.TrimSpace(bytesToString(e.Comm[:]))
	status := connectStatusFromRet(e.Ret)

	// Only report success and in_progress
	if status == "failed" {
		return
	}

	cmdLine := ""
	if exePath, err := readProcExe(e.Pid); err == nil {
		cmdLine = strings.TrimSpace(exePath)
	}

	l.sendEvent(Event{
		EventID:          l.nextEventID(),
		HostID:           l.hostID,
		Hostname:         l.hostname,
		Timestamp:        time.Now().UnixMilli(),
		EventType:        "network_connect",
		ProcessName:      comm,
		PID:              int(e.Pid),
		UID:              int(e.Uid),
		CommandLine:      cmdLine,
		SrcIP:            srcIP,
		DstIP:            dstIP,
		SrcPort:          srcPort,
		DstPort:          dstPort,
		Protocol:         protocolName(e.Protocol),
		ConnectStatus:    status,
		ReturnCode:       e.Ret,
		RemoteAddr:       fmt.Sprintf("%s:%d", dstIP, dstPort),
		NetworkDirection: "outbound",
	})
}

func (l *Loader) processAcceptEvent(data []byte) {
	// NOTE: accept_event BPF struct is layout-compatible with conn_event (tcp_connect).
	// Changes to either struct must be mirrored.
	e, err := parseConnEvent(data)
	if err != nil {
		logger.Debug("Failed to parse accept event", zap.Error(err))
		return
	}

	srcIP, dstIP, srcPort, dstPort := parseConnAddr(e)
	comm := strings.TrimSpace(bytesToString(e.Comm[:]))

	logger.Debug("[eBPF] Accept event captured",
		zap.String("src_ip", srcIP),
		zap.Uint16("src_port", srcPort),
		zap.String("dst_ip", dstIP),
		zap.Uint16("dst_port", dstPort),
		zap.String("comm", comm),
		zap.Uint32("pid", e.Pid))

	cmdLine := ""
	if exePath, err := readProcExe(e.Pid); err == nil {
		cmdLine = strings.TrimSpace(exePath)
	}

	// Capture process tree at event time (before short-lived process exits)
	processTreeJSON := l.captureProcessTreeJSON(int(e.Pid))

	l.sendEvent(Event{
		EventID:          l.nextEventID(),
		HostID:           l.hostID,
		Hostname:         l.hostname,
		Timestamp:        time.Now().UnixMilli(),
		EventType:        "network_accept",
		ProcessName:      comm,
		PID:              int(e.Pid),
		UID:              int(e.Uid),
		CommandLine:      cmdLine,
		SrcIP:            srcIP,
		DstIP:            dstIP,
		SrcPort:          srcPort,
		DstPort:          dstPort,
		Protocol:         protocolName(e.Protocol),
		ConnectStatus:    "success",
		ReturnCode:       0,
		RemoteAddr:       fmt.Sprintf("%s:%d", dstIP, dstPort),
		NetworkDirection: "inbound",
		ProcessTreeJSON:  processTreeJSON,
	})
}

// captureProcessTreeJSON captures process tree at event time (before short-lived process exits)
func (l *Loader) captureProcessTreeJSON(pid int) string {
	if pid <= 0 {
		return ""
	}
	tree, err := l.toolManager.GetProcessTree(pid)
	if err != nil {
		logger.Debug("Failed to get process tree at event time", zap.Int("pid", pid), zap.Error(err))
		return ""
	}
	data, err := json.Marshal(tree)
	if err != nil {
		logger.Debug("Failed to marshal process tree", zap.Int("pid", pid), zap.Error(err))
		return ""
	}
	return string(data)
}

func (l *Loader) sendEvent(event Event) {
	select {
	case l.eventChan <- event:
	default:
		dropped := atomic.AddUint64(&l.dropCount, 1)
		if dropped%1000 == 1 {
			logger.Warn("Event channel full, events being dropped",
				zap.Uint64("total_dropped", dropped),
				zap.String("last_type", event.EventType),
				zap.Int("last_pid", event.PID))
		}
	}
}

func (l *Loader) DroppedCount() uint64 {
	return atomic.LoadUint64(&l.dropCount)
}

func (l *Loader) nextEventID() string {
	seq := atomic.AddUint64(&l.seq, 1)
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), seq)
}

func argvBytesToCommandLine(data []byte) string {
	if fixed := argvFixedSlotsToCommandLine(data, execArgSlotLen); fixed != "" {
		return fixed
	}

	fields := make([]string, 0, 8)
	start := 0
	for i, b := range data {
		if b != 0 {
			continue
		}
		if i > start {
			fields = append(fields, string(data[start:i]))
		} else if len(fields) > 0 {
			break
		}
		start = i + 1
	}
	if start < len(data) {
		tail := strings.TrimRight(string(data[start:]), "\x00")
		if tail != "" {
			fields = append(fields, tail)
		}
	}
	return strings.Join(fields, " ")
}

func argvFixedSlotsToCommandLine(data []byte, slotLen int) string {
	if slotLen <= 0 || len(data) < slotLen*2 {
		return ""
	}

	firstNul := -1
	for i := 0; i < slotLen; i++ {
		if data[i] == 0 {
			firstNul = i
			break
		}
	}
	if firstNul < 0 || data[slotLen] == 0 {
		return ""
	}
	for i := firstNul + 1; i < slotLen; i++ {
		if data[i] != 0 {
			return ""
		}
	}

	fields := make([]string, 0, len(data)/slotLen)
	for offset := 0; offset+slotLen <= len(data); offset += slotLen {
		arg := bytesToString(data[offset : offset+slotLen])
		if arg == "" {
			break
		}
		fields = append(fields, arg)
	}
	return strings.Join(fields, " ")
}

func normalizeCommandLine(cmdline string) string {
	return strings.TrimSpace(strings.ReplaceAll(cmdline, "\x00", " "))
}

func processNameFromExec(comm, filename, cmdLine string) string {
	if filename != "" {
		if base := filepath.Base(filename); base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if cmdLine != "" {
		parts := strings.Fields(cmdLine)
		if len(parts) > 0 {
			if base := filepath.Base(parts[0]); base != "." && base != string(filepath.Separator) {
				return base
			}
		}
	}
	return comm
}

func readProcCmdline(pid uint32) (string, error) {
	procCmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", err
	}
	return normalizeCommandLine(string(bytes.ReplaceAll(procCmdline, []byte{0}, []byte(" ")))), nil
}

func readProcExe(pid uint32) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
}

// Close unloads all eBPF programs and closes resources
func (l *Loader) Close() {
	close(l.done)
	for _, rd := range l.readers {
		rd.Close()
	}
	for _, ln := range l.links {
		ln.Close()
	}
	for _, coll := range l.collections {
		coll.Close()
	}
	logger.Info("eBPF loader closed")
}
