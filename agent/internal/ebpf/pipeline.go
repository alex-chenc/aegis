package ebpf

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"aegis-agent/internal/logger"
	"aegis-agent/internal/monitor"
	"aegis-agent/internal/sigma"
	"aegis-agent/internal/tools"
	pb "aegis-agent/pkg/api/v1"

	"go.uber.org/zap"
)

type EventReporter interface {
	ReportEvents(events []*pb.RuntimeEvent) error
}

type Pipeline struct {
	collector   *Collector
	ruleLoader  *sigma.Loader
	reporter    EventReporter
	hostID      string
	hostname    string
	metrics     *monitor.Metrics
	seq         uint64
	flushEvery  time.Duration
	toolManager *tools.ToolManager
}

func NewPipeline(collector *Collector, ruleLoader *sigma.Loader, reporter EventReporter, hostID string, metrics *monitor.Metrics) *Pipeline {
	hostname, _ := os.Hostname()
	if metrics == nil {
		metrics = monitor.NewMetrics()
	}
	return &Pipeline{
		collector:   collector,
		ruleLoader:  ruleLoader,
		reporter:    reporter,
		hostID:      hostID,
		hostname:    hostname,
		metrics:     metrics,
		flushEvery:  2 * time.Second,
		toolManager: tools.NewToolManager(),
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

	logger.Debug("Event captured",
		zap.String("type", event.EventType),
		zap.String("cmd", event.CommandLine),
		zap.Int("pid", event.PID))

	// Debug logging for network events
	if event.EventType == "network_accept" || event.EventType == "network_connect" {
		logger.Debug("[NetworkEvent] Processing network event",
			zap.String("type", event.EventType),
			zap.String("src_ip", event.SrcIP),
			zap.Uint16("src_port", event.SrcPort),
			zap.String("dst_ip", event.DstIP),
			zap.Uint16("dst_port", event.DstPort),
			zap.String("protocol", event.Protocol),
			zap.String("direction", event.NetworkDirection),
			zap.String("category", fmt.Sprintf("%v", eventMap["category"])))
	}

	// Debug logging for file access events
	if event.EventType == "file_access" {
		logger.Info("[FileEvent] Processing file access event",
			zap.String("path", event.FilePath),
			zap.String("action", event.FileAction),
			zap.String("flags", event.FileFlags),
			zap.String("process", event.ProcessName),
			zap.Int("pid", event.PID),
			zap.String("category", fmt.Sprintf("%v", eventMap["category"])),
			zap.String("target_filename", fmt.Sprintf("%v", eventMap["TargetFilename"])),
			zap.String("target_filename_lower", fmt.Sprintf("%v", eventMap["targetfilename"])),
			zap.String("event_action", fmt.Sprintf("%v", eventMap["event.action"])))
	}

	matches := p.ruleLoader.MatchAll(eventMap)
	if len(matches) == 0 {
		if event.EventType == "file_access" {
			logger.Info("[FileEvent] No Sigma rules matched",
				zap.String("path", event.FilePath),
				zap.String("action", event.FileAction))
		}
		if event.EventType == "network_accept" || event.EventType == "network_connect" {
			logger.Debug("[NetworkEvent] No Sigma rules matched",
				zap.String("type", event.EventType),
				zap.String("src_ip", event.SrcIP),
				zap.Uint16("src_port", event.SrcPort),
				zap.String("dst_ip", event.DstIP),
				zap.Uint16("dst_port", event.DstPort),
				zap.String("protocol", event.Protocol))
		}
		return batch
	}

	for _, match := range matches {
		p.metrics.IncrMatched()
		logger.Info("Rule matched",
			zap.String("rule_id", match.ID),
			zap.String("title", match.Title),
			zap.String("mitre_id", match.MitreID),
			zap.String("severity", match.Severity))
		batch = append(batch, p.buildRuntimeEvent(event, match, eventMap))
	}

	return batch
}

func (p *Pipeline) buildEventMap(event Event) map[string]any {
	cmdLine := event.CommandLine
	if cmdLine == "" || cmdLine == " " {
		cmdLine = event.ProcessName
	}
	if event.FilePath != "" && cmdLine == event.ProcessName {
		cmdLine = event.FilePath + " " + cmdLine
	}

	exePath := event.FilePath
	if exePath == "" && cmdLine != "" {
		parts := strings.Fields(cmdLine)
		if len(parts) > 0 {
			exePath = parts[0]
		}
	}

	eventMap := map[string]any{
		"event_type":     event.EventType,
		"pid":            event.PID,
		"ppid":           event.PPID,
		"uid":            event.UID,
		"process_name":   event.ProcessName,
		"comm":           event.ProcessName,
		"commandline":    cmdLine,
		"image":          exePath,
		"exe":            exePath,
		"file_path":      event.FilePath,
		"args_truncated": event.ArgsTruncated,
		"ProcessName":    event.ProcessName,
		"CommandLine":    cmdLine,
		"Image":          exePath,
		"process.command_line": cmdLine,
	}

	switch event.EventType {
	case "process_exec", "process_fork", "process_exit":
		eventMap["category"] = "process_creation"

	case "file_access":
		eventMap["category"] = "file_event"
		eventMap["file_path"] = event.FilePath
		eventMap["filepath"] = event.FilePath
		eventMap["TargetFilename"] = event.FilePath
		eventMap["targetfilename"] = event.FilePath
		eventMap["file.path"] = event.FilePath
		eventMap["file_name"] = event.FileName
		eventMap["FileName"] = event.FileName
		eventMap["file.name"] = event.FileName
		eventMap["file_dir"] = event.FileDir
		eventMap["file.directory"] = event.FileDir
		eventMap["file_action"] = event.FileAction
		eventMap["event.action"] = event.FileAction
		eventMap["file_flags"] = event.FileFlags
		eventMap["open_flags"] = event.FileFlags
		eventMap["old_file_path"] = event.OldFilePath

	case "network_connect", "network_accept":
		eventMap["category"] = "network_connection"
		eventMap["src_ip"] = event.SrcIP
		eventMap["source_ip"] = event.SrcIP
		eventMap["SourceIp"] = event.SrcIP
		eventMap["sourceip"] = event.SrcIP
		eventMap["source.ip"] = event.SrcIP
		eventMap["src_port"] = event.SrcPort
		eventMap["source_port"] = event.SrcPort
		eventMap["SourcePort"] = event.SrcPort
		eventMap["sourceport"] = event.SrcPort
		eventMap["source.port"] = event.SrcPort
		eventMap["dst_ip"] = event.DstIP
		eventMap["destination_ip"] = event.DstIP
		eventMap["DestinationIp"] = event.DstIP
		eventMap["destinationip"] = event.DstIP
		eventMap["destination.ip"] = event.DstIP
		eventMap["dst_port"] = event.DstPort
		eventMap["destination_port"] = event.DstPort
		eventMap["DestinationPort"] = event.DstPort
		eventMap["destinationport"] = event.DstPort
		eventMap["destination.port"] = event.DstPort
		eventMap["network_transport"] = event.Protocol
		eventMap["network.transport"] = event.Protocol
		eventMap["Protocol"] = event.Protocol
		eventMap["network_direction"] = event.NetworkDirection
		eventMap["connect_status"] = event.ConnectStatus
		eventMap["return_code"] = event.ReturnCode
		eventMap["remote_addr"] = event.RemoteAddr

	case "privilege_change":
		eventMap["category"] = "privilege_escalation"
	}

	return eventMap
}

func (p *Pipeline) buildRuntimeEvent(event Event, match *sigma.CompiledRule, eventMap map[string]any) *pb.RuntimeEvent {
	// Use pre-captured process tree from event time (for short-lived processes)
	// Fall back to fetching at pipeline time if not pre-captured
	processTreeJSON := event.ProcessTreeJSON
	if processTreeJSON == "" && p.toolManager != nil && event.PID > 0 {
		processTreeJSON = p.getProcessTreeJSON(event.PID)
	}

	eventDataJSON := ""
	if data, err := json.Marshal(eventMap); err == nil {
		eventDataJSON = string(data)
	}

	cmdLine := event.CommandLine
	filePath := event.FilePath
	remoteAddr := event.RemoteAddr

	switch event.EventType {
	case "file_access":
		if cmdLine == "" {
			cmdLine = event.FilePath
		}
	case "network_connect", "network_accept":
		if cmdLine == "" {
			cmdLine = event.RemoteAddr
		}
	}

	return &pb.RuntimeEvent{
		EventId:          p.nextEventID(),
		HostId:           event.HostID,
		Hostname:         event.Hostname,
		Timestamp:        event.Timestamp,
		EventType:        event.EventType,
		ProcessName:      event.ProcessName,
		Pid:              int32(event.PID),
		Ppid:             int32(event.PPID),
		Uid:              int32(event.UID),
		CommandLine:      cmdLine,
		FilePath:         filePath,
		RemoteAddr:       remoteAddr,
		MatchedRuleId:    match.ID,
		MitreId:          match.MitreID,
		Severity:         match.Severity,
		ProcessTree:      processTreeJSON,
		MatchedRuleTitle: match.Title,
		EventDataJson:    eventDataJSON,
	}
}

func (p *Pipeline) getProcessTreeJSON(pid int) string {
	tree, err := p.toolManager.GetProcessTree(pid)
	if err != nil {
		logger.Debug("Failed to get process tree", zap.Int("pid", pid), zap.Error(err))
		return ""
	}
	data, err := json.Marshal(tree)
	if err != nil {
		logger.Debug("Failed to marshal process tree", zap.Error(err))
		return ""
	}
	return string(data)
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
