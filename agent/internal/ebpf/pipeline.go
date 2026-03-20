package ebpf

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"aegis-agent/internal/logger"
	"aegis-agent/internal/monitor"
	"aegis-agent/internal/sigma"
	pb "aegis-agent/pkg/api/v1"

	"go.uber.org/zap"
)

type EventReporter interface {
	ReportEvents(events []*pb.RuntimeEvent) error
}

type Pipeline struct {
	collector  *Collector
	ruleLoader *sigma.Loader
	reporter   EventReporter
	hostID     string
	hostname   string
	metrics    *monitor.Metrics
	seq        uint64
	flushEvery time.Duration
}

func NewPipeline(collector *Collector, ruleLoader *sigma.Loader, reporter EventReporter, hostID string, metrics *monitor.Metrics) *Pipeline {
	hostname, _ := os.Hostname()
	if metrics == nil {
		metrics = monitor.NewMetrics()
	}
	return &Pipeline{
		collector:  collector,
		ruleLoader: ruleLoader,
		reporter:   reporter,
		hostID:     hostID,
		hostname:   hostname,
		metrics:    metrics,
		flushEvery: 2 * time.Second,
	}
}

func (p *Pipeline) Run(done <-chan struct{}) {
	flushTicker := time.NewTicker(p.flushEvery)
	defer flushTicker.Stop()

	batch := make([]*pb.RuntimeEvent, 0, 32)

	for {
		select {
		case <-done:
			for {
				select {
				case event := <-p.collector.Events():
					batch = p.appendMatchedEvents(batch, event)
				default:
					if len(batch) > 0 {
						p.flush(batch)
					}
					return
				}
			}
		case event := <-p.collector.Events():
			batch = p.appendMatchedEvents(batch, event)
		case <-flushTicker.C:
			if len(batch) == 0 {
				continue
			}
			p.flush(batch)
			batch = nil
		}
	}
}

func (p *Pipeline) appendMatchedEvents(batch []*pb.RuntimeEvent, event Event) []*pb.RuntimeEvent {
	p.metrics.IncrEvents()
	eventMap := p.buildEventMap(event)

	matches := p.ruleLoader.MatchAll(eventMap)
	if len(matches) == 0 {
		return batch
	}

	for _, match := range matches {
		p.metrics.IncrMatched()
		batch = append(batch, p.buildRuntimeEvent(event, match))
	}

	return batch
}

func (p *Pipeline) buildEventMap(event Event) map[string]any {
	eventMap := map[string]any{
		"event_type":   event.EventType,
		"pid":          event.PID,
		"ppid":         event.PPID,
		"uid":          event.UID,
		"process_name": event.ProcessName,
		"commandline":  event.CommandLine,
		"image":        event.CommandLine,
		"exe":          event.CommandLine,
		"comm":         event.ProcessName,
		"file_path":    event.FilePath,
		"remote_addr":  event.RemoteAddr,
	}

	switch event.EventType {
	case "process_exec", "process_fork", "process_exit":
		eventMap["category"] = "process_creation"
	case "file_access":
		eventMap["category"] = "file_event"
	case "network_connect":
		eventMap["category"] = "network_connection"
	case "privilege_change":
		eventMap["category"] = "privilege_escalation"
	}

	return eventMap
}

func (p *Pipeline) buildRuntimeEvent(event Event, match *sigma.CompiledRule) *pb.RuntimeEvent {
	return &pb.RuntimeEvent{
		EventId:       p.nextEventID(),
		HostId:        event.HostID,
		Hostname:      event.Hostname,
		Timestamp:     event.Timestamp,
		EventType:     event.EventType,
		ProcessName:   event.ProcessName,
		Pid:           int32(event.PID),
		Ppid:          int32(event.PPID),
		Uid:           int32(event.UID),
		CommandLine:   event.CommandLine,
		FilePath:      event.FilePath,
		RemoteAddr:    event.RemoteAddr,
		MatchedRuleId: match.ID,
		MitreId:       match.MitreID,
		Severity:      match.Severity,
	}
}

func (p *Pipeline) nextEventID() string {
	seq := atomic.AddUint64(&p.seq, 1)
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), seq)
}

func (p *Pipeline) flush(events []*pb.RuntimeEvent) {
	if err := p.reporter.ReportEvents(events); err != nil {
		logger.Error("Failed to report events",
			zap.Int("count", len(events)),
			zap.Error(err))
		return
	}

	logger.Debug("Events reported", zap.Int("count", len(events)))
}
