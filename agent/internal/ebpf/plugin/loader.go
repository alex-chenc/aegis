package plugin

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"aegis-agent/internal/logger"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
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
	Events map[int]EventDef
}

type EventDef struct {
	Name   string
	Fields map[int]FieldDef
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
			if fieldDef, ok := eventDef.Fields[int(fieldID)]; ok {
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
type PluginInstance struct {
	mu         sync.Mutex
	Collection *ebpf.Collection
	Links      []link.Link
	Reader     *ringbuf.Reader
	PackageID  string
	Done       chan struct{}
}

var (
	instancesMu sync.Mutex
	instances   = make(map[string]*PluginInstance)
)

// LoadPlugin loads an eBPF .bpf.o file, attaches hooks, and starts reading events.
func LoadPlugin(pkg *PackageInfo, artifactPath string, onEvent func(pkgID string, event map[string]interface{})) error {
	instancesMu.Lock()
	defer instancesMu.Unlock()

	if _, exists := instances[pkg.PackageID]; exists {
		return fmt.Errorf("plugin %s already loaded", pkg.PackageID)
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

	// Find and open the ringbuf map for events
	for mapName, m := range coll.Maps {
		if m.Type() == ebpf.RingBuf {
			reader, err := ringbuf.NewReader(m)
			if err != nil {
				logger.Warn("failed to open ringbuf", zap.String("map", mapName), zap.Error(err))
				continue
			}
			inst.Reader = reader
			logger.Info("ringbuf reader opened", zap.String("map", mapName), zap.String("package_id", pkg.PackageID))

			// Start event reading goroutine
			go readEvents(inst, pkg, onEvent)
			break
		}
	}

	instances[pkg.PackageID] = inst

	logger.Info("eBPF plugin loaded",
		zap.String("package_id", pkg.PackageID),
		zap.String("artifact", bpfObjPath),
		zap.Int("hooks_attached", len(inst.Links)),
		zap.Bool("has_reader", inst.Reader != nil),
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

	close(inst.Done)

	if inst.Reader != nil {
		inst.Reader.Close()
	}

	for _, l := range inst.Links {
		l.Close()
	}

	if inst.Collection != nil {
		inst.Collection.Close()
	}

	delete(instances, packageID)

	logger.Info("eBPF plugin unloaded", zap.String("package_id", packageID))
	return nil
}

func findBPFObject(extractDir, transport string) (string, error) {
	pluginDir := filepath.Join(extractDir, "plugin")
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return "", fmt.Errorf("read plugin dir: %w", err)
	}

	// Prefer the transport-specific artifact
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".bpf.o" {
			if transport != "" && (containsStr(name, transport) || containsStr(name, "plugin")) {
				return filepath.Join(pluginDir, name), nil
			}
		}
	}

	// Fallback: any .bpf.o
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bpf.o" {
			return filepath.Join(pluginDir, e.Name()), nil
		}
	}

	return "", fmt.Errorf("no .bpf.o found in %s", pluginDir)
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
		return nil, fmt.Errorf("invalid tracepoint format: %s (expected category:name)", hook.Attach)
	case "uprobe":
		return nil, fmt.Errorf("uprobe not yet supported")
	default:
		return nil, fmt.Errorf("unsupported attach type: %s", hook.AttachType)
	}
}

func readEvents(inst *PluginInstance, pkg *PackageInfo, onEvent func(pkgID string, event map[string]interface{})) {
	for {
		select {
		case <-inst.Done:
			return
		default:
		}

		record, err := inst.Reader.Read()
		if err != nil {
			if err.Error() == "ringbuf reader closed" {
				return
			}
			logger.Error("ringbuf read error", zap.String("package_id", pkg.PackageID), zap.Error(err))
			continue
		}

		decoded, err := DecodeEnvelope(record.RawSample)
		if err != nil {
			logger.Error("decode envelope error", zap.String("package_id", pkg.PackageID), zap.Error(err))
			continue
		}

		evt := map[string]interface{}{
			"package_id": pkg.PackageID,
			"timestamp":  int64(decoded.TimestampNS),
			"pid":        int(decoded.PID),
			"tid":        int(decoded.TID),
			"uid":        int(decoded.UID),
			"gid":        int(decoded.GID),
			"event_type": fmt.Sprintf("%d", decoded.EventType),
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

		if onEvent != nil {
			onEvent(pkg.PackageID, evt)
		}
	}
}

func splitAttach(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
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
