package ebpf

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

// Event represents a security event from eBPF
type Event struct {
	EventID       string
	HostID        string
	Hostname      string
	Timestamp     int64
	EventType     string
	ProcessName   string
	PID           int
	PPID          int
	UID           int
	CommandLine   string
	FilePath      string
	RemoteAddr    string
	ArgsTruncated bool
}

// Collector manages eBPF event collection
type Collector struct {
	hostID   string
	hostname string
	events   chan Event
	done     chan struct{}
	mu       sync.RWMutex
	loader   *Loader
	running  bool
}

// NewCollector creates a new event collector
func NewCollector(hostID string, bufferSize int) *Collector {
	hostname, _ := os.Hostname()
	if bufferSize <= 0 {
		bufferSize = 10000
	}

	return &Collector{
		hostID:   hostID,
		hostname: hostname,
		events:   make(chan Event, bufferSize),
		done:     make(chan struct{}),
	}
}

// Start initializes eBPF and begins event collection
func (c *Collector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("collector already running")
	}

	// Create eBPF loader
	loader, err := NewLoader(c.hostID, c.events)
	if err != nil {
		logger.Warn("Failed to create eBPF loader, falling back to /proc monitor",
			zap.Error(err))
		go c.monitorProc()
		return nil
	}

	// Load all eBPF programs
	if err := loader.LoadAll(); err != nil {
		logger.Warn("Failed to load eBPF programs, falling back to /proc monitor",
			zap.Error(err))
		go c.monitorProc()
		return nil
	}

	c.loader = loader
	c.running = true

	logger.Info("Event collector started with eBPF",
		zap.String("host_id", c.hostID),
		zap.String("hostname", c.hostname))

	return nil
}

// Events returns the event channel
func (c *Collector) Events() <-chan Event {
	return c.events
}

// Stop halts event collection
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}

	if c.loader != nil {
		c.loader.Close()
		c.loader = nil
	}

	c.running = false
	logger.Info("Event collector stopped")
}

// IsRunning returns whether the collector is running
func (c *Collector) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// monitorProc is a fallback method that monitors /proc
func (c *Collector) monitorProc() {
	logger.Info("Starting /proc fallback monitor")

	knownPIDs := make(map[int]struct{})
	c.snapshotExistingProcesses(knownPIDs)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
		}

		entries, err := os.ReadDir("/proc")
		if err != nil {
			continue
		}

		for _, entry := range entries {
			pid, err := parsePID(entry.Name())
			if err != nil {
				continue
			}

			if _, ok := knownPIDs[pid]; ok {
				continue
			}
			knownPIDs[pid] = struct{}{}

			comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
			cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
			cmdlineStr := string(bytes.ReplaceAll(cmdline, []byte{0}, []byte(" ")))
			cmdlineStr = strings.TrimSpace(cmdlineStr)

			event := Event{
				EventID:     generateEventID(),
				HostID:      c.hostID,
				Hostname:    c.hostname,
				Timestamp:   time.Now().UnixMilli(),
				EventType:   "process_exec",
				PID:         pid,
				ProcessName: strings.TrimSpace(string(comm)),
				CommandLine: cmdlineStr,
			}

			select {
			case c.events <- event:
			default:
			}
		}

		// Clean up exited processes
		for pid := range knownPIDs {
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); os.IsNotExist(err) {
				delete(knownPIDs, pid)
			}
		}
	}
}

// snapshotExistingProcesses captures all currently running processes
// to ensure we don't miss processes that were already running before monitoring started.
func (c *Collector) snapshotExistingProcesses(knownPIDs map[int]struct{}) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		logger.Warn("Failed to read /proc for initial snapshot", zap.Error(err))
		return
	}

	count := 0
	for _, entry := range entries {
		pid, err := parsePID(entry.Name())
		if err != nil {
			continue
		}

		knownPIDs[pid] = struct{}{}

		comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdlineStr := string(bytes.ReplaceAll(cmdline, []byte{0}, []byte(" ")))
		cmdlineStr = strings.TrimSpace(cmdlineStr)

		if cmdlineStr == "" {
			continue // Skip kernel threads
		}

		event := Event{
			EventID:     generateEventID(),
			HostID:      c.hostID,
			Hostname:    c.hostname,
			Timestamp:   time.Now().UnixMilli(),
			EventType:   "process_exec",
			PID:         pid,
			ProcessName: strings.TrimSpace(string(comm)),
			CommandLine: cmdlineStr,
		}

		select {
		case c.events <- event:
			count++
		default:
			logger.Warn("Event channel full during snapshot, dropping event",
				zap.Int("pid", pid))
		}
	}

	logger.Info("Initial /proc snapshot complete",
		zap.Int("total_pids", len(knownPIDs)),
		zap.Int("events_sent", count))
}

func parsePID(name string) (int, error) {
	pid := 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return 0, fmt.Errorf("not a pid")
		}
		pid = pid*10 + int(name[i]-'0')
	}
	return pid, nil
}

func generateEventID() string {
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000000)
}
