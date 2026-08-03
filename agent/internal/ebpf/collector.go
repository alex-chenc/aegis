package ebpf

import (
	"fmt"
	"os"
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
	MonotonicNS   uint64
	EventType     string
	ProcessName   string
	PID           int
	PPID          int
	UID           int
	GID           int
	CommandLine   string
	FilePath      string
	Image         string
	ArgsTruncated bool

	// File event fields
	FileName    string
	FileDir     string
	FileAction  string
	FileFlags   string
	OldFilePath string

	// Network event fields
	SrcIP            string
	DstIP            string
	SrcPort          uint16
	DstPort          uint16
	Protocol         string
	NetworkDirection string
	ConnectStatus    string
	ReturnCode       int32
	RemoteAddr       string

	// Agent Guard monitor fields. These are syscall metadata only; no
	// stdin/stdout, file content, network payload, or environment is captured.
	SecurityCategory   string
	SecurityOperation  string
	SecurityTarget     string
	SecuritySecondary  string
	SecurityArg0       uint64
	SecurityArg1       uint64
	SecurityArg2       uint64
	SyscallReturn      int64
	SecurityDecision   string
	SecurityPolicySlot uint64
	SecurityRuleSlot   uint64

	// Process tree JSON (captured at event time for short-lived processes)
	ProcessTreeJSON string
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
	options  LoaderOptions
}

// NewCollector creates a new event collector
func NewCollector(hostID string, bufferSize int) *Collector {
	return NewCollectorWithOptions(hostID, bufferSize, LoaderOptions{})
}

func NewCollectorWithOptions(hostID string, bufferSize int, options LoaderOptions) *Collector {
	hostname, _ := os.Hostname()
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	return &Collector{
		hostID:   hostID,
		hostname: hostname,
		events:   make(chan Event, bufferSize),
		done:     make(chan struct{}),
		options:  options,
	}
}

// Start initializes eBPF and begins event collection.
// Returns nil if eBPF is not available — agent continues without event engine.
func (c *Collector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("collector already running")
	}

	loader, err := NewLoaderWithOptions(c.hostID, c.events, c.options)
	if err != nil {
		logger.Warn("[eBPF] event engine disabled",
			zap.Error(err),
			zap.String("note", "agent other functions remain enabled"))
		return nil
	}

	if err := loader.LoadAll(); err != nil {
		loader.Close()
		logger.Warn("[eBPF] event engine disabled due to load failure",
			zap.Error(err),
			zap.String("note", "agent other functions remain enabled"))
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

func parsePID(name string) (int, error) {
	if len(name) == 0 {
		return 0, fmt.Errorf("empty pid")
	}
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
