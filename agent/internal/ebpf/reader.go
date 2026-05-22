package ebpf

import (
	"fmt"
	"os"

	"aegis-agent/internal/ebpf/kernel"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
)

// EventReader abstracts reading raw events from eBPF maps.
type EventReader interface {
	Read() ([]byte, error)
	Close() error
}

// RingbufEventReader reads from a BPF_MAP_TYPE_RINGBUF.
type RingbufEventReader struct {
	rd *ringbuf.Reader
}

func NewRingbufEventReader(m *ebpf.Map) (*RingbufEventReader, error) {
	rd, err := ringbuf.NewReader(m)
	if err != nil {
		return nil, fmt.Errorf("ringbuf new reader: %w", err)
	}
	return &RingbufEventReader{rd: rd}, nil
}

func (r *RingbufEventReader) Read() ([]byte, error) {
	record, err := r.rd.Read()
	if err != nil {
		return nil, err
	}
	return record.RawSample, nil
}

func (r *RingbufEventReader) Close() error {
	return r.rd.Close()
}

// PerfEventReader reads from a BPF_MAP_TYPE_PERF_EVENT_ARRAY.
type PerfEventReader struct {
	rd *perf.Reader
}

func NewPerfEventReader(m *ebpf.Map) (*PerfEventReader, error) {
	rd, err := perf.NewReader(m, os.Getpagesize()*16)
	if err != nil {
		return nil, fmt.Errorf("perf new reader: %w", err)
	}
	return &PerfEventReader{rd: rd}, nil
}

func (r *PerfEventReader) Read() ([]byte, error) {
	record, err := r.rd.Read()
	if err != nil {
		return nil, err
	}
	return record.RawSample, nil
}

func (r *PerfEventReader) Close() error {
	return r.rd.Close()
}

// NewEventReader creates the appropriate EventReader based on kernel capabilities.
func NewEventReader(m *ebpf.Map, caps *kernel.Capabilities) (EventReader, error) {
	switch caps.Transport {
	case kernel.TransportRingbuf:
		return NewRingbufEventReader(m)
	case kernel.TransportPerf:
		return NewPerfEventReader(m)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", caps.Transport)
	}
}

// BPFObjectSuffix returns the file suffix for the given transport.
func BPFObjectSuffix(caps *kernel.Capabilities) string {
	switch caps.Transport {
	case kernel.TransportRingbuf:
		return ".ringbuf.bpf.o"
	case kernel.TransportPerf:
		return ".perf.bpf.o"
	default:
		return ".bpf.o"
	}
}
