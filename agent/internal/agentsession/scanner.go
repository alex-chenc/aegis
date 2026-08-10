package agentsession

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CursorStore interface {
	Load(identity string) (FileCursor, bool)
	Save(identity string, cursor FileCursor) error
}

type MemoryCursorStore struct {
	mu      sync.RWMutex
	cursors map[string]FileCursor
}

// JSONCursorStore keeps only file identity and offsets locally. It is written
// with mode 0600 and never contains session text.
type JSONCursorStore struct {
	mu      sync.Mutex
	path    string
	cursors map[string]FileCursor
}

func NewJSONCursorStore(path string) (*JSONCursorStore, error) {
	store := &JSONCursorStore{path: path, cursors: make(map[string]FileCursor)}
	if path == "" {
		return store, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &store.cursors); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *JSONCursorStore) Load(identity string) (FileCursor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cursor, ok := s.cursors[identity]
	return cursor, ok
}

func (s *JSONCursorStore) Save(identity string, cursor FileCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors[identity] = cursor
	if s.path == "" {
		return nil
	}
	data, err := json.Marshal(s.cursors)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func NewMemoryCursorStore() *MemoryCursorStore {
	return &MemoryCursorStore{cursors: make(map[string]FileCursor)}
}

func (s *MemoryCursorStore) Load(identity string) (FileCursor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cursor, ok := s.cursors[identity]
	return cursor, ok
}

func (s *MemoryCursorStore) Save(identity string, cursor FileCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors[identity] = cursor
	return nil
}

type Scanner struct {
	cfg      ScanConfig
	store    CursorStore
	redactor *Redactor
	now      func() time.Time
	mu       sync.Mutex
	lease    bool
}

func NewScanner(cfg ScanConfig, store CursorStore, redactor *Redactor) *Scanner {
	cfg = cfg.withDefaults()
	if store == nil {
		store = NewMemoryCursorStore()
	}
	if redactor == nil {
		redactor = NewRedactor()
	}
	return &Scanner{cfg: cfg, store: store, redactor: redactor, now: time.Now}
}

func (s *Scanner) Scan(ctx context.Context) (ScanResult, error) {
	s.mu.Lock()
	if s.lease {
		s.mu.Unlock()
		return ScanResult{}, ErrScanAlreadyRunning
	}
	s.lease = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.lease = false
		s.mu.Unlock()
	}()

	start := s.now()
	result := ScanResult{CursorUpdates: make(map[string]FileCursor)}
	for _, root := range s.cfg.Roots {
		if err := s.scanRoot(ctx, root, start, &result); err != nil {
			return result, err
		}
		if result.BudgetExhausted {
			break
		}
	}
	sort.Slice(result.Sessions, func(i, j int) bool {
		if result.Sessions[i].Source != result.Sessions[j].Source {
			return result.Sessions[i].Source < result.Sessions[j].Source
		}
		return result.Sessions[i].SessionID < result.Sessions[j].SessionID
	})
	return result, nil
}

var ErrScanAlreadyRunning = scanError("agent session scan already running")

type scanError string

func (e scanError) Error() string { return string(e) }

func (s *Scanner) scanRoot(ctx context.Context, root SourceRoot, start time.Time, result *ScanResult) error {
	if root.Root == "" || (root.Source != SourceClaude && root.Source != SourceCodex) {
		return nil
	}
	rootPath := filepath.Clean(root.Root)
	cutoff := start.Add(-s.cfg.InitialLookback)
	return filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if start.Add(s.cfg.MaxScanDuration).Before(s.now()) || result.FilesDiscovered >= s.cfg.MaxFiles || result.BytesRead >= s.cfg.MaxNewBytes {
			result.BudgetExhausted = true
			return filepath.SkipAll
		}
		depth := strings.Count(strings.TrimPrefix(path, rootPath), string(os.PathSeparator))
		if entry.IsDir() {
			if depth > s.cfg.MaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		result.FilesDiscovered++
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.ModTime().Before(cutoff) {
			return nil
		}
		if root.UID >= 0 && fileUID(info) != root.UID {
			return nil
		}
		identity := sourceIdentity(root, path, info)
		cursor, _ := s.store.Load(identity)
		parsed, err := parseJSONLFile(path, root.Source, cursor, s.cfg, s.redactor)
		if err != nil {
			return nil
		}
		result.FilesProcessed++
		result.BytesRead += parsed.BytesRead
		if parsed.Unsupported {
			result.UnsupportedFiles++
		}
		for _, delta := range parsed.Sessions {
			result.Sessions = append(result.Sessions, *delta)
		}
		result.CursorUpdates[identity] = parsed.Cursor
		if result.BytesRead >= s.cfg.MaxNewBytes {
			result.BudgetExhausted = true
		}
		return nil
	})
}

// Commit persists cursor updates only after the caller has durably accepted
// the corresponding session batch. This prevents a transport outage from
// advancing the cursor past unreported content.
func (s *Scanner) Commit(result ScanResult) error {
	for identity, cursor := range result.CursorUpdates {
		if err := s.store.Save(identity, cursor); err != nil {
			return err
		}
	}
	return nil
}

func sourceIdentity(root SourceRoot, path string, info os.FileInfo) string {
	return string(root.Source) + ":" + filepath.Clean(root.Root) + ":" + path + ":" + fileIdentity(path, info)
}

func fileUID(info os.FileInfo) int {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1
	}
	return int(stat.Uid)
}
