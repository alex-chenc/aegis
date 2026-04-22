package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	Message   string `json:"message"`
}

// QueryHistoricalLogs queries historical logs within a time range
func (m *ToolManager) QueryHistoricalLogs(args map[string]interface{}) (interface{}, error) {
	startTimeStr, ok := args["start_time"].(string)
	if !ok {
		return nil, fmt.Errorf("start_time is required")
	}

	endTimeStr, ok := args["end_time"].(string)
	if !ok {
		return nil, fmt.Errorf("end_time is required")
	}

	filter, _ := args["filter"].(string)

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time format, use RFC3339")
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time format, use RFC3339")
	}

	logDirs := []string{
		"/var/log",
		"/var/log/syslog",
		"/var/log/messages",
	}

	logs := []LogEntry{}

	for _, logDir := range logDirs {
		entries, err := m.searchLogs(logDir, startTime, endTime, filter)
		if err == nil {
			logs = append(logs, entries...)
		}
	}

	if len(logs) > 1000 {
		logs = logs[:1000]
	}

	return map[string]interface{}{
		"logs":     logs,
		"count":    len(logs),
		"captured": time.Now().Unix(),
	}, nil
}

func (m *ToolManager) searchLogs(dir string, start, end time.Time, filter string) ([]LogEntry, error) {
	var logs []LogEntry

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Log but don't fail - some directories may not be accessible
		return logs, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(start) {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".log" && ext != "" && !strings.Contains(entry.Name(), "log.") {
			continue
		}

		entries, err := m.readLogFile(path, start, end, filter)
		if err == nil {
			logs = append(logs, entries...)
		}
	}

	return logs, nil
}

func (m *ToolManager) readLogFile(path string, start, end time.Time, filter string) ([]LogEntry, error) {
	// Resolve symlinks and verify path safety
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}

	// Verify path is within allowed directory
	realPath = filepath.Clean(realPath)
	if !strings.HasPrefix(realPath, "/var/log") && !strings.HasPrefix(realPath, "/var/log/") {
		return nil, fmt.Errorf("path outside allowed directory: %s", realPath)
	}

	file, err := os.Open(realPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []LogEntry
	scanner := bufio.NewScanner(file)

	// Set buffer for long lines (max 1MB per line)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	// Pre-compile filter for performance
	var filterLower string
	if filter != "" {
		filterLower = strings.ToLower(filter)
	}

	maxLines := 10000
	lineCount := 0

	for scanner.Scan() {
		lineCount++
		if lineCount > maxLines {
			break
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		if filterLower != "" && !strings.Contains(strings.ToLower(line), filterLower) {
			continue
		}

		logs = append(logs, LogEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Source:    path,
			Message:   line,
		})
	}

	return logs, nil
}