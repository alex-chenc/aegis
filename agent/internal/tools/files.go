package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OpenFile represents an open file descriptor
type OpenFile struct {
	PID     int    `json:"pid"`
	FD      string `json:"fd"`
	Type    string `json:"type"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
}

// GetOpenFiles gets open files for a process
func (m *ToolManager) GetOpenFiles(args map[string]interface{}) (interface{}, error) {
	pid, err := toInt(args["pid"])
	if err != nil {
		return nil, fmt.Errorf("invalid pid parameter: %w", err)
	}

	// Check if process exists
	procPath := fmt.Sprintf("/proc/%d", pid)
	if _, err := os.Stat(procPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("process %d not found", pid)
	}

	fdPath := filepath.Join(procPath, "fd")
	entries, err := os.ReadDir(fdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fd directory: %w", err)
	}

	files := []OpenFile{}
	for _, entry := range entries {
		fd := entry.Name()
		target, err := os.Readlink(filepath.Join(fdPath, fd))
		if err != nil {
			continue
		}

		// Get file info
		var mode, fileType string
		if info, err := os.Stat(target); err == nil {
			mode = info.Mode().String()
			if info.IsDir() {
				fileType = "directory"
			} else {
				fileType = "file"
			}
		} else {
			// Might be a socket or other special file
			if strings.HasPrefix(target, "socket:") {
				fileType = "socket"
			} else if strings.HasPrefix(target, "pipe:") {
				fileType = "pipe"
			} else if strings.HasPrefix(target, "/dev/") {
				fileType = "device"
			} else {
				fileType = "unknown"
			}
			mode = "unknown"
		}

		files = append(files, OpenFile{
			PID:  pid,
			FD:   fd,
			Type: fileType,
			Path: target,
			Mode: mode,
		})
	}

	if len(files) > 1000 {
		files = files[:1000]
	}

	return map[string]interface{}{
		"pid":   pid,
		"files": files,
		"count": len(files),
	}, nil
}