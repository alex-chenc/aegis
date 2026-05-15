package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"aegis-agent/internal/logger"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"go.uber.org/zap"
)

// Loader manages eBPF programs and ring buffer readers
type Loader struct {
	collections map[string]*ebpf.Collection
	links       []link.Link
	readers     map[string]*ringbuf.Reader
	eventChan   chan Event
	hostID      string
	hostname    string
	seq         uint64
	done        chan struct{}
	dropCount   uint64 // atomic counter for dropped events
}

type execRuntimeReaders struct {
	readCmdline func(pid uint32) (string, error)
	readExe     func(pid uint32) (string, error)
}

type bpfProgramConfig struct {
	name       string
	tracepoint string
	category   string
	mapName    string
	required   bool
}

type loadBPFProgramFunc func(name, tracepoint, category, mapName string) error

// NewLoader creates a new eBPF loader
func NewLoader(hostID string, eventChan chan Event) (*Loader, error) {
	hostname, _ := os.Hostname()
	return &Loader{
		collections: make(map[string]*ebpf.Collection),
		readers:     make(map[string]*ringbuf.Reader),
		eventChan:   eventChan,
		hostID:      hostID,
		hostname:    hostname,
		done:        make(chan struct{}),
	}, nil
}

// LoadAll loads eBPF programs for process events (exec/fork).
func (l *Loader) LoadAll() error {
	return l.loadConfiguredPrograms(defaultBPFPrograms(), l.loadProgram)
}

func defaultBPFPrograms() []bpfProgramConfig {
	return []bpfProgramConfig{
		{name: "execve", tracepoint: "sys_enter_execve", category: "syscalls", mapName: "exec_events", required: true},
		{name: "fork", tracepoint: "sched_process_fork", category: "sched", mapName: "fork_events"},
	}
}

func (l *Loader) loadConfiguredPrograms(programs []bpfProgramConfig, load loadBPFProgramFunc) error {
	for _, prog := range programs {
		if err := load(prog.name, prog.tracepoint, prog.category, prog.mapName); err != nil {
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

func (l *Loader) loadProgram(name, tracepoint, category, mapName string) error {
	// Get the directory of the executable to find BPF objects
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	// Try multiple paths for BPF objects
	objPaths := []string{
		filepath.Join(execDir, "bpf", name+".bpf.o"),
		filepath.Join(execDir, "..", "internal", "ebpf", "bpf", "obj", name+".bpf.o"),
		filepath.Join("internal/ebpf/bpf/obj", name+".bpf.o"),
	}

	var spec *ebpf.CollectionSpec
	for _, objPath := range objPaths {
		spec, err = ebpf.LoadCollectionSpec(objPath)
		if err == nil {
			break
		}
	}

	if spec == nil {
		return fmt.Errorf("failed to load spec for %s: tried paths %v, last error: %w", name, objPaths, err)
	}

	// Load collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to load collection for %s: %w", name, err)
	}
	l.collections[name] = coll

	// Attach to tracepoint
	tp, err := link.Tracepoint(category, tracepoint, coll.Programs["trace_"+name], nil)
	if err != nil {
		coll.Close()
		return fmt.Errorf("failed to attach tracepoint for %s: %w", name, err)
	}
	l.links = append(l.links, tp)

	// Create ring buffer reader
	rd, err := ringbuf.NewReader(coll.Maps[mapName])
	if err != nil {
		return fmt.Errorf("failed to create ringbuf reader for %s: %w", name, err)
	}
	l.readers[name] = rd

	// Start reader goroutine
	go l.readEvents(name, rd)

	logger.Info("eBPF program loaded",
		zap.String("program", name),
		zap.String("tracepoint", tracepoint))

	return nil
}

func (l *Loader) readEvents(name string, rd *ringbuf.Reader) {
	logger.Info("Ring buffer reader started", zap.String("program", name))
	for {
		select {
		case <-l.done:
			return
		default:
		}

		logger.Debug("Waiting for ringbuffer event", zap.String("program", name))
		record, err := rd.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				return
			}
			logger.Debug("Ring buffer read error",
				zap.String("program", name),
				zap.Error(err))
			continue
		}

		logger.Debug("Ringbuffer event received",
			zap.String("program", name),
			zap.Int("size", len(record.RawSample)))
		l.processEvent(name, record.RawSample)
	}
}

func (l *Loader) processEvent(name string, data []byte) {
	switch name {
	case "execve":
		l.processExecEvent(data)
	case "fork":
		l.processForkEvent(data)
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

	event := Event{
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
	}

	l.sendEvent(event)
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

// DroppedCount returns the total number of dropped events
func (l *Loader) DroppedCount() uint64 {
	return atomic.LoadUint64(&l.dropCount)
}

func (l *Loader) nextEventID() string {
	seq := atomic.AddUint64(&l.seq, 1)
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), seq)
}

func argvBytesToCommandLine(data []byte) string {
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
