package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
}

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

// LoadAll loads all eBPF programs
func (l *Loader) LoadAll() error {
	programs := []struct {
		name       string
		tracepoint string
		category   string
		mapName    string
	}{
		{"execve", "sched/sched_process_exec", "sched", "exec_events"},
		{"fork", "sched/sched_process_fork", "sched", "fork_events"},
		{"exit", "sched/sched_process_exit", "sched", "exit_events"},
		{"openat", "syscalls/sys_enter_openat", "syscalls", "file_events"},
		{"connect", "syscalls/sys_enter_connect", "syscalls", "conn_events"},
		{"setuid", "syscalls/sys_enter_setuid", "syscalls", "priv_events"},
		{"setgid", "syscalls/sys_enter_setgid", "syscalls", "priv_events"},
		{"capset", "syscalls/sys_enter_capset", "syscalls", "cap_events"},
	}

	for _, prog := range programs {
		if err := l.loadProgram(prog.name, prog.tracepoint, prog.category, prog.mapName); err != nil {
			logger.Warn("Failed to load eBPF program",
				zap.String("program", prog.name),
				zap.Error(err))
			continue
		}
	}

	return nil
}

func (l *Loader) loadProgram(name, tracepoint, category, mapName string) error {
	// Load BPF object from filesystem
	objPath := filepath.Join("internal/ebpf/bpf/obj", name+".bpf.o")
	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return fmt.Errorf("failed to load spec for %s: %w", name, err)
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
	for {
		select {
		case <-l.done:
			return
		default:
		}

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

		l.processEvent(name, record.RawSample)
	}
}

func (l *Loader) processEvent(name string, data []byte) {
	switch name {
	case "execve":
		l.processExecEvent(data)
	case "fork":
		l.processForkEvent(data)
	case "exit":
		l.processExitEvent(data)
	case "openat":
		l.processFileEvent(data)
	case "connect":
		l.processConnEvent(data)
	case "setuid", "setgid":
		l.processPrivEvent(name, data)
	case "capset":
		l.processCapEvent(data)
	}
}

func (l *Loader) processExecEvent(data []byte) {
	var e ExecEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "process_exec",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		PPID:        int(e.Ppid),
		UID:         int(e.Uid),
		CommandLine: bytesToString(e.Filename[:]) + " " + bytesToString(e.Args[:]),
		FilePath:    bytesToString(e.Filename[:]),
	}

	l.sendEvent(event)
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

func (l *Loader) processExitEvent(data []byte) {
	var e ExitEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "process_exit",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		UID:         int(e.Uid),
		CommandLine: fmt.Sprintf("exit code %d", e.ExitCode),
	}

	l.sendEvent(event)
}

func (l *Loader) processFileEvent(data []byte) {
	var e FileEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "file_access",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		UID:         int(e.Uid),
		FilePath:    bytesToString(e.Filename[:]),
		CommandLine: fmt.Sprintf("flags=%d", e.Flags),
	}

	l.sendEvent(event)
}

func (l *Loader) processConnEvent(data []byte) {
	var e ConnEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	daddr := net.IP(e.Daddr[:])

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "network_connect",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		UID:         int(e.Uid),
		RemoteAddr:  fmt.Sprintf("%s:%d", daddr.String(), e.Dport),
		CommandLine: fmt.Sprintf("connect to %s:%d", daddr.String(), e.Dport),
	}

	l.sendEvent(event)
}

func (l *Loader) processPrivEvent(name string, data []byte) {
	var e PrivEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	syscallName := bytesToString(e.Syscall[:])
	if syscallName == "" {
		syscallName = name
	}

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "privilege_change",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		UID:         int(e.Uid),
		CommandLine: fmt.Sprintf("%s target_uid=%d", syscallName, e.TargetUID),
	}

	l.sendEvent(event)
}

func (l *Loader) processCapEvent(data []byte) {
	var e CapEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &e); err != nil {
		return
	}

	event := Event{
		EventID:     l.nextEventID(),
		HostID:      l.hostID,
		Hostname:    l.hostname,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "privilege_change",
		ProcessName: bytesToString(e.Comm[:]),
		PID:         int(e.Pid),
		UID:         int(e.Uid),
		CommandLine: fmt.Sprintf("capset effective=%x permitted=%x", e.CapEffective, e.CapPermitted),
	}

	l.sendEvent(event)
}

func (l *Loader) sendEvent(event Event) {
	select {
	case l.eventChan <- event:
	default:
		logger.Warn("Event channel full, dropping event",
			zap.String("type", event.EventType))
	}
}

func (l *Loader) nextEventID() string {
	seq := atomic.AddUint64(&l.seq, 1)
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), seq)
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
