package agentsession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Spool is a bounded, mode-0600 JSON queue for already-redacted session
// batches. It intentionally omits project paths before writing to disk.
type Spool struct {
	mu       sync.Mutex
	path     string
	capacity int
}

func NewSpool(path string, capacity int) *Spool {
	if capacity < 1 {
		capacity = 100
	}
	return &Spool{path: path, capacity: capacity}
}
func (s *Spool) Append(deltas []SessionDelta) error {
	if s == nil || s.path == "" || len(deltas) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.load()
	if err != nil {
		return err
	}
	for _, delta := range deltas {
		delta.ProjectPath = ""
		pending = append(pending, delta)
	}
	if len(pending) > s.capacity {
		pending = pending[len(pending)-s.capacity:]
	}
	return s.save(pending)
}
func (s *Spool) Drain(ctx context.Context, reporter Reporter) error {
	if s == nil || s.path == "" || reporter == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.load()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	if err := reporter.ReportSessionDeltas(ctx, pending); err != nil {
		return err
	}
	return s.save(nil)
}
func (s *Spool) load() ([]SessionDelta, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SessionDelta
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode agent session spool: %w", err)
	}
	return out, nil
}
func (s *Spool) save(value []SessionDelta) error {
	if len(value) == 0 {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
