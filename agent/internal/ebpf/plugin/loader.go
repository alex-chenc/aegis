package plugin

import (
	"encoding/json"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
	"sync"

	"aegis-agent/internal/logger"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"go.uber.org/zap"
)

const (
	AEGIS_PLUGIN_PAYLOAD_MAX = 256
	AEGIS_TLV_STRING         = 1
	AEGIS_TLV_INT32          = 2
	AEGIS_TLV_UINT32         = 3
	AEGIS_TLV_INT64          = 4
	AEGIS_TLV_UINT64         = 5
	AEGIS_TLV_BOOL           = 6
	AEGIS_TLV_BYTES          = 7
)

// PluginHook mirrors dynpkg.PluginHook to avoid import cycle
type PluginHook struct {
	Name       string
	AttachType string
	Attach     string
	Program    string
}

// PluginManifest mirrors the relevant parts of dynpkg.Manifest
type PluginManifest struct {
	Hooks       []PluginHook
	EventSchema EventSchema
}

type EventSchema struct {
	Events map[string]EventDef
}

type EventDef struct {
	Name   string
	Fields map[string]FieldDef
}

type FieldDef struct {
	Name string
	Type string
}

// PackageInfo holds the minimal info needed by the plugin loader
type PackageInfo struct {
	PackageID      string
	ActiveArtifact string
	Manifest       *PluginManifest
}

type PluginEventEnvelope struct {
	TimestampNS  uint64
	PluginIDHash uint32
	EventType    uint32
	PID          uint32
	TID          uint32
	UID          uint32
	GID          uint32
	PayloadLen   uint32
	Payload      [AEGIS_PLUGIN_PAYLOAD_MAX]byte
}

type DecodedPluginEvent struct {
	PackageID string
	PluginID  string
	EventName string
	Timestamp int64
	PID       int
	TID       int
	UID       int
	GID       int
	Fields    map[string]interface{}
}

func DecodeEnvelope(raw []byte) (*PluginEventEnvelope, error) {
	// Header: TimestampNS(8) + PluginIDHash(4) + EventType(4) + PID(4) + TID(4) + UID(4) + GID(4) + PayloadLen(4) = 36 bytes
	if len(raw) < 36 {
		return nil, fmt.Errorf("event too short: %d bytes, need at least 36", len(raw))
	}

	env := &PluginEventEnvelope{}
	env.TimestampNS = binary.LittleEndian.Uint64(raw[0:8])
	env.PluginIDHash = binary.LittleEndian.Uint32(raw[8:12])
	env.EventType = binary.LittleEndian.Uint32(raw[12:16])
	env.PID = binary.LittleEndian.Uint32(raw[16:20])
	env.TID = binary.LittleEndian.Uint32(raw[20:24])
	env.UID = binary.LittleEndian.Uint32(raw[24:28])
	env.GID = binary.LittleEndian.Uint32(raw[28:32])
	env.PayloadLen = binary.LittleEndian.Uint32(raw[32:36])

	// Copy only the actual payload data (up to PayloadLen or max size)
	payloadStart := 36
	payloadEnd := payloadStart + int(env.PayloadLen)
	if payloadEnd > len(raw) {
		payloadEnd = len(raw)
	}
	if payloadEnd > payloadStart+AEGIS_PLUGIN_PAYLOAD_MAX {
		payloadEnd = payloadStart + AEGIS_PLUGIN_PAYLOAD_MAX
	}
	if payloadStart < payloadEnd {
		copy(env.Payload[:], raw[payloadStart:payloadEnd])
	}

	return env, nil
}

func DecodeTLV(payload []byte, schema EventSchema) (map[string]interface{}, error) {
	fields := make(map[string]interface{})
	offset := 0

	for offset < len(payload) {
		if offset+4 > len(payload) {
			break
		}

		fieldID := binary.LittleEndian.Uint16(payload[offset : offset+2])
		fieldLen := binary.LittleEndian.Uint16(payload[offset+2 : offset+4])
		offset += 4

		if offset+int(fieldLen) > len(payload) {
			break
		}

		value := payload[offset : offset+int(fieldLen)]
		offset += int(fieldLen)

		for _, eventDef := range schema.Events {
			if fieldDef, ok := eventDef.Fields[fmt.Sprintf("%d", fieldID)]; ok {
				fields[fieldDef.Name] = decodeValue(value, fieldDef.Type)
				break
			}
		}
	}

	return fields, nil
}

func decodeValue(data []byte, fieldType string) interface{} {
	switch fieldType {
	case "string":
		return string(data)
	case "int32":
		if len(data) >= 4 {
			return int32(binary.LittleEndian.Uint32(data))
		}
	case "uint32":
		if len(data) >= 4 {
			return binary.LittleEndian.Uint32(data)
		}
	case "int64":
		if len(data) >= 8 {
			return int64(binary.LittleEndian.Uint64(data))
		}
	case "uint64":
		if len(data) >= 8 {
			return binary.LittleEndian.Uint64(data)
		}
	case "bool":
		return len(data) > 0 && data[0] != 0
	}
	return nil
}

// PluginInstance holds the runtime state of a loaded eBPF plugin
type EventReader interface {
	Read() (rawSample []byte, err error)
	Close() error
}

type ringbufReader struct {
	r *ringbuf.Reader
}

func (r *ringbufReader) Read() ([]byte, error) {
	rec, err := r.r.Read()
	if err != nil {
		return nil, err
	}
	return rec.RawSample, nil
}

func (r *ringbufReader) Close() error {
	return r.r.Close()
}

type perfReader struct {
	r *perf.Reader
}

func (r *perfReader) Read() ([]byte, error) {
	rec, err := r.r.Read()
	if err != nil {
		return nil, err
	}
	return rec.RawSample, nil
}

func (r *perfReader) Close() error {
	return r.r.Close()
}

type PluginInstance struct {
	mu         sync.Mutex
	Collection *ebpf.Collection
	Links      []link.Link
	Readers    []EventReader
	PackageID  string
	Transport  string
	Done       chan struct{}
}

var (
	instancesMu sync.Mutex
	instances   = make(map[string]*PluginInstance)
)

// LoadPlugin loads an eBPF .bpf.o file, attaches hooks, and starts reading events.
// If a plugin with the same package ID is already loaded, it is unloaded first.
func LoadPlugin(pkg *PackageInfo, artifactPath string, onEvent func(pkgID string, event map[string]interface{})) error {
	instancesMu.Lock()
	defer instancesMu.Unlock()

	if existingInst, exists := instances[pkg.PackageID]; exists {
		logger.Warn("plugin already loaded, unloading old instance before reload",
			zap.String("package_id", pkg.PackageID),
		)
		unloadInstance(existingInst)
		delete(instances, pkg.PackageID)
	}

	// Clean up any orphaned eBPF programs from previous agent runs before loading new ones.
	// This prevents duplicate programs from being attached to the same tracepoints.
	if pkg.Manifest != nil {
		var progNames []string
		for _, hook := range pkg.Manifest.Hooks {
			name := hook.Program
			if name == "" {
				name = hook.Name
			}
			progNames = append(progNames, name)
		}
		CleanupOrphanedPrograms(progNames)
	}

	// Find the .bpf.o file
	bpfObjPath, err := findBPFObject(artifactPath, pkg.ActiveArtifact)
	if err != nil {
		return fmt.Errorf("find bpf object: %w", err)
	}

	// Load the eBPF collection
	coll, err := ebpf.LoadCollection(bpfObjPath)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	inst := &PluginInstance{
		Collection: coll,
		PackageID:  pkg.PackageID,
		Done:       make(chan struct{}),
	}

	// Attach each hook from the plugin manifest
	for _, hook := range pkg.Manifest.Hooks {
		progName := hook.Program
		if progName == "" {
			progName = hook.Name
		}

		prog := coll.Programs[progName]
		if prog == nil {
			// Fallback: try to find by section name (e.g. "tp/syscalls/sys_enter_socket")
			sectionName := hook.AttachType + "/" + hook.Attach
			prog = coll.Programs[sectionName]
			if prog != nil {
				logger.Info("program found by section name fallback",
					zap.String("requested_program", progName),
					zap.String("section_name", sectionName),
					zap.String("package_id", pkg.PackageID),
				)
			}
		}
		if prog == nil {
			logger.Warn("program not found in collection",
				zap.String("program", progName),
				zap.String("package_id", pkg.PackageID),
			)
			continue
		}

		l, err := attachHook(prog, hook)
		if err != nil {
			logger.Error("failed to attach hook",
				zap.String("hook", hook.Name),
				zap.String("attach_type", hook.AttachType),
				zap.String("attach", hook.Attach),
				zap.Error(err),
			)
			continue
		}
		inst.Links = append(inst.Links, l)
		logger.Info("hook attached",
			zap.String("hook", hook.Name),
			zap.String("attach_type", hook.AttachType),
			zap.String("attach", hook.Attach),
		)
	}

	// Find and open ALL ringbuf/perf maps for events
	readerCount := 0
	for mapName, m := range coll.Maps {
		if m.Type() == ebpf.RingBuf {
			reader, err := ringbuf.NewReader(m)
			if err == nil {
				r := &ringbufReader{r: reader}
				inst.Readers = append(inst.Readers, r)
				inst.Transport = "ringbuf"
				logger.Info("ringbuf reader opened", zap.String("map", mapName), zap.String("package_id", pkg.PackageID))
				go readEvents(r, inst, pkg, onEvent)
				readerCount++
			} else {
				logger.Warn("ringbuf open failed",
					zap.String("map", mapName),
					zap.Error(err),
				)
			}
		}
		if m.Type() == ebpf.PerfEventArray {
			const perfPageSize = 4096
			reader, err := perf.NewReader(m, perfPageSize)
			if err == nil {
				r := &perfReader{r: reader}
				inst.Readers = append(inst.Readers, r)
				inst.Transport = "perf"
				logger.Info("perf buffer reader opened", zap.String("map", mapName), zap.String("package_id", pkg.PackageID))
				go readEvents(r, inst, pkg, onEvent)
				readerCount++
			} else {
				logger.Warn("perf buffer open failed",
					zap.String("map", mapName),
					zap.Error(err),
				)
			}
		}
	}

	logger.Info("all event readers opened", zap.Int("reader_count", readerCount), zap.String("package_id", pkg.PackageID))

	instances[pkg.PackageID] = inst

	logger.Info("eBPF plugin loaded",
		zap.String("package_id", pkg.PackageID),
		zap.String("artifact", bpfObjPath),
		zap.Int("hooks_attached", len(inst.Links)),
		zap.Int("readers", len(inst.Readers)),
	)
	return nil
}

// UnloadPlugin unloads an eBPF plugin and cleans up resources.
func UnloadPlugin(packageID string) error {
	instancesMu.Lock()
	defer instancesMu.Unlock()

	inst, exists := instances[packageID]
	if !exists {
		return nil
	}

	unloadInstance(inst)
	delete(instances, packageID)

	logger.Info("eBPF plugin unloaded", zap.String("package_id", packageID))
	return nil
}

// unloadInstance closes all readers, links, and the collection for a plugin instance.
// Caller must hold instancesMu.
func unloadInstance(inst *PluginInstance) {
	close(inst.Done)

	for _, reader := range inst.Readers {
		reader.Close()
	}

	for _, l := range inst.Links {
		l.Close()
	}

	if inst.Collection != nil {
		inst.Collection.Close()
	}
}

// CleanupOrphanedPrograms finds and closes any eBPF programs whose names match the
// given programNames list. This handles the case where a previous agent run left
// programs attached to tracepoints (e.g. due to an unclean shutdown).
// This should be called at agent startup, before loading any new plugins.
func CleanupOrphanedPrograms(programNames []string) {
	if len(programNames) == 0 {
		return
	}
	nameSet := make(map[string]struct{}, len(programNames))
	for _, n := range programNames {
		nameSet[n] = struct{}{}
	}

	// Use bpftool to list all loaded programs and close any matching ours.
	out, err := exec.Command("bpftool", "prog", "list", "--json").Output()
	if err != nil {
		logger.Debug("bpftool prog list failed (orphan cleanup skipped)", zap.Error(err))
		return
	}

	type bpftoolProg struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	var programs []bpftoolProg
	if err := json.Unmarshal(out, &programs); err != nil {
		logger.Debug("failed to parse bpftool output (orphan cleanup skipped)", zap.Error(err))
		return
	}

	orphanCount := 0
	for _, p := range programs {
		if _, ok := nameSet[p.Name]; !ok {
			continue
		}
		prog, err := ebpf.NewProgramFromID(ebpf.ProgramID(p.ID))
		if err != nil {
			// Program may have already been cleaned up by the kernel when old process exited.
			logger.Debug("could not open orphaned program (may already be gone)",
				zap.Int("id", p.ID), zap.String("name", p.Name), zap.Error(err))
			continue
		}
		prog.Close()
		orphanCount++
		logger.Info("closed orphaned eBPF program from previous run",
			zap.Int("id", p.ID), zap.String("name", p.Name))
	}

	if orphanCount > 0 {
		logger.Info("orphaned eBPF program cleanup complete", zap.Int("closed", orphanCount))
	}
}

func findBPFObject(extractDir, transport string) (string, error) {
	pluginDir := filepath.Join(extractDir, "plugin")
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return "", fmt.Errorf("read plugin dir: %w", err)
	}

	logger.Debug("findBPFObject scanning directory",
		zap.String("pluginDir", pluginDir),
		zap.String("transport", transport),
		zap.Int("entries", len(entries)),
	)

	// Prefer the transport-specific artifact
	// NOTE: filepath.Ext() only returns the last extension (e.g. ".o" for ".bpf.o"),
	// so we use strings.HasSuffix to match the double extension ".bpf.o".
	for _, e := range entries {
		name := e.Name()
		logger.Debug("findBPFObject entry", zap.String("name", name), zap.Bool("hasBpfO", strings.HasSuffix(name, ".bpf.o")))
		if strings.HasSuffix(name, ".bpf.o") {
			if transport != "" && (containsStr(name, transport) || containsStr(name, "plugin")) {
				logger.Debug("findBPFObject matched transport-specific", zap.String("path", filepath.Join(pluginDir, name)))
				return filepath.Join(pluginDir, name), nil
			}
		}
	}

	// Fallback: any .bpf.o
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bpf.o") {
			logger.Debug("findBPFObject fallback match", zap.String("path", filepath.Join(pluginDir, e.Name())))
			return filepath.Join(pluginDir, e.Name()), nil
		}
	}

	return "", fmt.Errorf("no .bpf.o found in %s (entries: %v)", pluginDir, entryNames(entries))
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func attachHook(prog *ebpf.Program, hook PluginHook) (link.Link, error) {
	switch hook.AttachType {
	case "kprobe":
		return link.Kprobe(hook.Attach, prog, nil)
	case "kretprobe":
		return link.Kretprobe(hook.Attach, prog, nil)
	case "tracepoint":
		// Attach format: "category:name"
		parts := splitAttach(hook.Attach)
		if len(parts) == 2 {
			return link.Tracepoint(parts[0], parts[1], prog, nil)
		}
		return nil, fmt.Errorf("invalid tracepoint format: %s (expected category:name or category/name)", hook.Attach)
	case "uprobe":
		return nil, fmt.Errorf("uprobe not yet supported")
	default:
		return nil, fmt.Errorf("unsupported attach type: %s", hook.AttachType)
	}
}

func readEvents(reader EventReader, inst *PluginInstance, pkg *PackageInfo, onEvent func(pkgID string, event map[string]interface{})) {
	for {
		select {
		case <-inst.Done:
			return
		default:
		}

		rawSample, err := reader.Read()
		if err != nil {
			select {
			case <-inst.Done:
				return
			default:
			}
			logger.Error("event read error", zap.String("package_id", pkg.PackageID), zap.String("transport", inst.Transport), zap.Error(err))
			continue
		}

		logger.Debug("plugin event received",
			zap.String("package_id", pkg.PackageID),
			zap.Int("sample_len", len(rawSample)),
		)

		decoded, err := DecodeEnvelope(rawSample)
		if err != nil {
			logger.Error("decode envelope error", zap.String("package_id", pkg.PackageID), zap.Error(err), zap.Int("sample_len", len(rawSample)))
			continue
		}

		logger.Debug("plugin event decoded",
			zap.String("package_id", pkg.PackageID),
			zap.Uint32("event_type", decoded.EventType),
			zap.Uint32("pid", decoded.PID),
			zap.Uint32("payload_len", decoded.PayloadLen),
		)

		evt := map[string]interface{}{
			"package_id": pkg.PackageID,
			"category":   "kernel_plugin",
			"timestamp":  int64(decoded.TimestampNS),
			"pid":        int(decoded.PID),
			"tid":        int(decoded.TID),
			"uid":        int(decoded.UID),
			"gid":        int(decoded.GID),
			"event_type": fmt.Sprintf("%d", decoded.EventType),
		}

		// Map numeric event_type to string name from event_schema
		if pkg.Manifest != nil && pkg.Manifest.EventSchema.Events != nil {
			eventKey := fmt.Sprintf("%d", decoded.EventType)
			if eventDef, ok := pkg.Manifest.EventSchema.Events[eventKey]; ok {
				if eventDef.Name != "" {
					evt["event_type"] = eventDef.Name
					logger.Debug("event_type mapped",
						zap.String("package_id", pkg.PackageID),
						zap.String("from", eventKey),
						zap.String("to", eventDef.Name),
					)
				}
			}
		}

		if pkg.Manifest != nil && pkg.Manifest.EventSchema.Events != nil {
			payloadLen := int(decoded.PayloadLen)
			if payloadLen > AEGIS_PLUGIN_PAYLOAD_MAX {
				payloadLen = AEGIS_PLUGIN_PAYLOAD_MAX
			}
			if payloadLen > 0 {
				fields, err := DecodeTLV(decoded.Payload[:payloadLen], pkg.Manifest.EventSchema)
				if err == nil {
					for k, v := range fields {
						evt[k] = v
					}
				}
			}
		}

		// Skip events with PID 0 (kernel/system events)
	if decoded.PID == 0 {
		continue
	}

	if onEvent != nil {
		logger.Debug("calling onEvent", zap.String("package_id", pkg.PackageID), zap.String("event_type", fmt.Sprintf("%v", evt["event_type"])))
			onEvent(pkg.PackageID, evt)
		}
	}
}

func splitAttach(s string) []string {
	// Support both "category:name" and "category/name" formats
	for i := 0; i < len(s); i++ {
		if s[i] == ':' || s[i] == '/' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstr(s, sub))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
