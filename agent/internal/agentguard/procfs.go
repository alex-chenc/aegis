package agentguard

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProcessScanner interface {
	Scan() ([]ProcessSnapshot, error)
	ReadPID(pid uint32) (ProcessSnapshot, error)
	BootID() string
}

type ProcFSScanner struct {
	root        string
	configMu    sync.Mutex
	configCache map[uint32]configCacheEntry
}

type configCacheEntry struct {
	values    []string
	expiresAt time.Time
}

func NewProcFSScanner(root string) *ProcFSScanner {
	if root == "" {
		root = "/proc"
	}
	return &ProcFSScanner{root: root, configCache: make(map[uint32]configCacheEntry)}
}

func (s *ProcFSScanner) BootID() string {
	data, err := os.ReadFile(filepath.Join(s.root, "sys/kernel/random/boot_id"))
	if err != nil {
		return "unknown"
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "unknown"
	}
	return value
}

func (s *ProcFSScanner) Scan() ([]ProcessSnapshot, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read proc root: %w", err)
	}
	out := make([]ProcessSnapshot, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid64, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil || pid64 == 0 {
			continue
		}
		process, err := s.ReadPID(uint32(pid64))
		if err != nil {
			continue
		}
		if process.Exe == "" && len(process.Argv) == 0 {
			continue
		}
		out = append(out, process)
	}
	return out, nil
}

func (s *ProcFSScanner) ReadPID(pid uint32) (ProcessSnapshot, error) {
	if pid == 0 {
		return ProcessSnapshot{}, errors.New("pid is zero")
	}
	dir := filepath.Join(s.root, strconv.FormatUint(uint64(pid), 10))
	statData, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return ProcessSnapshot{}, fmt.Errorf("read proc stat: %w", err)
	}
	ppid, startTicks, err := parseProcStat(statData)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	process := ProcessSnapshot{
		Identity: ProcessIdentity{PID: pid, StartTicks: startTicks},
		PPID:     ppid,
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		for _, value := range bytes.Split(bytes.TrimRight(data, "\x00"), []byte{0}) {
			if len(value) > 0 {
				process.Argv = append(process.Argv, string(value))
			}
		}
	}
	if value, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
		process.Exe = value
	}
	if value, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
		process.CWD = value
	}
	if data, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
		process.UID, process.GID = parseProcStatus(data)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cgroup")); err == nil {
		process.CgroupPath = strings.TrimSpace(string(data))
		if info, ok := ParseContainerCgroup(process.CgroupPath); ok {
			process.ContainerID = info.ContainerID
		}
	}
	process.ConfigEvidence = s.configEvidence(process.UID)
	return process, nil
}

func (s *ProcFSScanner) configEvidence(uid uint32) []string {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	if cached, ok := s.configCache[uid]; ok && time.Now().Before(cached.expiresAt) {
		return append([]string(nil), cached.values...)
	}
	found := discoverConfigEvidence(uid)
	s.configCache[uid] = configCacheEntry{
		values: append([]string(nil), found...), expiresAt: time.Now().Add(time.Minute),
	}
	return found
}

func parseProcStat(data []byte) (uint32, uint64, error) {
	content := strings.TrimSpace(string(data))
	closing := strings.LastIndex(content, ")")
	if closing < 0 || closing+1 >= len(content) {
		return 0, 0, errors.New("proc stat format invalid")
	}
	fields := strings.Fields(content[closing+1:])
	if len(fields) < 20 {
		return 0, 0, errors.New("proc stat fields incomplete")
	}
	ppid, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("parse proc ppid: %w", err)
	}
	startTicks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || startTicks == 0 {
		return 0, 0, errors.New("parse proc start ticks")
	}
	return uint32(ppid), startTicks, nil
}

func parseProcStatus(data []byte) (uint32, uint32) {
	var uid, gid uint32
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "Uid":
			value, _ := strconv.ParseUint(fields[1], 10, 32)
			uid = uint32(value)
		case "Gid":
			value, _ := strconv.ParseUint(fields[1], 10, 32)
			gid = uint32(value)
		}
	}
	return uid, gid
}

func discoverConfigEvidence(uid uint32) []string {
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil || account.HomeDir == "" {
		return nil
	}
	return discoverConfigEvidenceInHome(account.HomeDir)
}

func discoverConfigEvidenceInHome(homeDir string) []string {
	var found []string
	for _, marker := range []string{
		".codex", ".openclaw", ".hermes", ".claude", ".config/opencode", ".gemini",
	} {
		path := filepath.Join(homeDir, marker)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			found = append(found, marker)
		}
	}
	return found
}
