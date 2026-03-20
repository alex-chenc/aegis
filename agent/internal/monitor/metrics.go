package monitor

import (
	"runtime"
	"sync/atomic"
	"time"
)

type Metrics struct {
	EventCount    uint64
	MatchedRules  uint64
	BlockExecuted uint64
	BlockFailed   uint64
	ToolCalls     uint64
	StartTime     time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{StartTime: time.Now()}
}

func (m *Metrics) IncrEvents()      { atomic.AddUint64(&m.EventCount, 1) }
func (m *Metrics) IncrMatched()     { atomic.AddUint64(&m.MatchedRules, 1) }
func (m *Metrics) IncrBlocks()      { atomic.AddUint64(&m.BlockExecuted, 1) }
func (m *Metrics) IncrBlockFailed() { atomic.AddUint64(&m.BlockFailed, 1) }
func (m *Metrics) IncrToolCalls()   { atomic.AddUint64(&m.ToolCalls, 1) }

func (m *Metrics) GetStats() map[string]interface{} {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return map[string]interface{}{
		"event_count":    atomic.LoadUint64(&m.EventCount),
		"matched_rules":  atomic.LoadUint64(&m.MatchedRules),
		"block_executed": atomic.LoadUint64(&m.BlockExecuted),
		"block_failed":   atomic.LoadUint64(&m.BlockFailed),
		"tool_calls":     atomic.LoadUint64(&m.ToolCalls),
		"uptime_seconds": time.Since(m.StartTime).Seconds(),
		"memory_mb":      mem.Alloc / 1024 / 1024,
		"goroutines":     runtime.NumGoroutine(),
	}
}
