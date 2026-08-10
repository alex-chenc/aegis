package agentguard

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const maxTrustedToolEventBytes = 64 * 1024

type ToolHookReceiver struct {
	path      string
	handler   func([]byte) (BehaviorEvent, error)
	listener  *net.UnixListener
	mu        sync.Mutex
	wg        sync.WaitGroup
	stopped   bool
	authorize func(string, uint32, uint32) bool
}

func StartToolHookReceiver(
	path string,
	policy ToolSocketRuntimePolicy,
	authorize func(string, uint32, uint32) bool,
	handler func([]byte) (BehaviorEvent, error),
) (*ToolHookReceiver, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) || path == "/" || len(path) > 100 || handler == nil || authorize == nil ||
		(policy.Mode != 0o600 && policy.Mode != 0o660) {
		return nil, errors.New("agent_guard_tool_hook_socket_path_invalid")
	}
	parent := filepath.Dir(path)
	if created, err := ensureTrustedSocketParent(parent); err != nil {
		return nil, errors.New("agent_guard_tool_hook_socket_parent_untrusted")
	} else if created {
		logger.Info("agent_guard_tool_hook_parent_created",
			zap.String("parent_path", parent),
			zap.Int("mode", 0o700))
	}
	if existing, err := os.Lstat(path); err == nil {
		if existing.Mode()&os.ModeSocket == 0 || !ownedByEUID(existing) {
			return nil, errors.New("agent_guard_tool_hook_socket_target_untrusted")
		}
		if err := os.Remove(path); err != nil {
			return nil, errors.New("agent_guard_tool_hook_socket_stale_remove_failed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("agent_guard_tool_hook_socket_target_untrusted")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, errors.New("agent_guard_tool_hook_socket_listen_failed")
	}
	if policy.GroupID != nil {
		if err := os.Chown(path, os.Geteuid(), int(*policy.GroupID)); err != nil {
			_ = listener.Close()
			_ = os.Remove(path)
			return nil, errors.New("agent_guard_tool_hook_socket_chown_failed")
		}
	}
	if err := os.Chmod(path, policy.Mode); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, errors.New("agent_guard_tool_hook_socket_chmod_failed")
	}
	receiver := &ToolHookReceiver{
		path: path, handler: handler, listener: listener, authorize: authorize,
	}
	receiver.wg.Add(1)
	go receiver.run()
	return receiver, nil
}

// ensureTrustedSocketParent makes the normal systemd/runtime-directory case
// self-healing without weakening the socket trust boundary. A missing parent
// is created only below an existing directory owned by the Agent user that is
// not writable by group/other. Existing parents still have to pass the same
// ownership and permission checks.
func ensureTrustedSocketParent(parent string) (bool, error) {
	info, err := os.Lstat(parent)
	if err == nil {
		if !info.IsDir() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
			return false, errors.New("socket parent is not trusted")
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	grandparent := filepath.Dir(parent)
	grandparentInfo, grandparentErr := os.Lstat(grandparent)
	if grandparentErr != nil || !grandparentInfo.IsDir() || grandparentInfo.Mode().Perm()&0o022 != 0 || !ownedByEUID(grandparentInfo) {
		return false, errors.New("socket grandparent is not trusted")
	}
	if err := os.Mkdir(parent, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return false, err
	}
	info, err = os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return false, errors.New("created socket parent is not trusted")
	}
	return true, nil
}

func (r *ToolHookReceiver) run() {
	defer r.wg.Done()
	for {
		connection, err := r.listener.AcceptUnix()
		if err != nil {
			r.mu.Lock()
			stopped := r.stopped
			r.mu.Unlock()
			if !stopped {
				logger.Warn("agent_guard_tool_hook_accept_failed",
					zap.String("error_code", "agent_guard_tool_hook_accept_failed"))
			}
			return
		}
		// Codex sends each lifecycle/tool callback over a short-lived Unix
		// connection. Processing these connections concurrently can reorder
		// SessionStart, Pre/PostToolUse, and SessionEnd at the manager even
		// though Codex emitted them in order. Keep ingress serialization here so
		// a late PostToolUse cannot be rejected merely because SessionEnd won a
		// scheduling race.
		r.wg.Add(1)
		r.handle(connection)
	}
}

func (r *ToolHookReceiver) handle(connection *net.UnixConn) {
	defer r.wg.Done()
	defer connection.Close()
	credential, ok := unixPeerCredential(connection)
	if !ok {
		logger.Warn("agent_guard_tool_hook_peer_rejected",
			zap.String("error_code", "agent_guard_tool_hook_peer_untrusted"))
		return
	}
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 4096), maxTrustedToolEventBytes)
	for scanner.Scan() {
		payload := append([]byte(nil), scanner.Bytes()...)
		if len(payload) == 0 {
			continue
		}
		var identity struct {
			SourceID string `json:"source_id"`
		}
		if json.Unmarshal(payload, &identity) != nil ||
			!r.authorize(identity.SourceID, credential.Uid, credential.Gid) {
			logger.Warn("agent_guard_tool_hook_peer_rejected",
				zap.String("error_code", "agent_guard_tool_hook_peer_unauthorized"))
			continue
		}
		if _, err := r.handler(payload); err != nil {
			logger.Warn("agent_guard_tool_hook_event_rejected",
				zap.String("error_code", errorCode(err)))
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("agent_guard_tool_hook_read_failed",
			zap.String("error_code", "agent_guard_tool_hook_read_failed"))
	}
}

func (r *ToolHookReceiver) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	listener := r.listener
	r.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	r.wg.Wait()
	if info, err := os.Lstat(r.path); err == nil && info.Mode()&os.ModeSocket != 0 && ownedByEUID(info) {
		_ = os.Remove(r.path)
	}
}

func unixPeerCredential(connection *net.UnixConn) (*unix.Ucred, bool) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return nil, false
	}
	var credential *unix.Ucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		credential, controlErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || controlErr != nil || credential == nil {
		return nil, false
	}
	return credential, credential.Pid > 0
}

func ownedByEUID(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
