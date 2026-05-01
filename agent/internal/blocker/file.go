package blocker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

type QuarantineRecord struct {
	OriginalPath   string `json:"original_path"`
	QuarantinePath string `json:"quarantine_path"`
	Timestamp      int64  `json:"timestamp"`
	Reason         string `json:"reason"`
}

func (b *Blocker) QuarantineFile(filePath string) error {
	if err := os.MkdirAll(b.quarantineDir, 0755); err != nil {
		return err
	}

	quarantinePath := filepath.Join(b.quarantineDir,
		fmt.Sprintf("%s.%d", filepath.Base(filePath), time.Now().Unix()))

	if err := moveFile(filePath, quarantinePath); err != nil {
		return fmt.Errorf("failed to quarantine %s: %w", filePath, err)
	}

	record := &QuarantineRecord{
		OriginalPath:   filePath,
		QuarantinePath: quarantinePath,
		Timestamp:      time.Now().Unix(),
	}
	b.saveQuarantineRecord(record)

	b.recordAudit("quarantine_file", filePath, quarantinePath, "success")
	logger.Info("File quarantined", zap.String("original", filePath), zap.String("quarantined", quarantinePath))
	return nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func (b *Blocker) RollbackQuarantine(quarantinePath string) error {
	record, err := b.findQuarantineRecord(quarantinePath)
	if err != nil {
		return err
	}

	if err := os.Rename(quarantinePath, record.OriginalPath); err != nil {
		return fmt.Errorf("failed to rollback: %w", err)
	}

	b.recordAudit("rollback_quarantine", quarantinePath, record.OriginalPath, "success")
	logger.Info("Quarantine rolled back", zap.String("from", quarantinePath), zap.String("to", record.OriginalPath))
	return nil
}

func (b *Blocker) saveQuarantineRecord(record *QuarantineRecord) {
	logFile := filepath.Join(b.quarantineDir, "quarantine_log.json")
	var records []QuarantineRecord

	data, _ := os.ReadFile(logFile)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &records)
	}

	records = append(records, *record)
	newData, _ := json.MarshalIndent(records, "", "  ")
	_ = os.WriteFile(logFile, newData, 0644)
}

func (b *Blocker) findQuarantineRecord(quarantinePath string) (*QuarantineRecord, error) {
	logFile := filepath.Join(b.quarantineDir, "quarantine_log.json")
	data, err := os.ReadFile(logFile)
	if err != nil {
		return nil, err
	}

	var records []QuarantineRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}

	for i := len(records) - 1; i >= 0; i-- {
		if records[i].QuarantinePath == quarantinePath {
			return &records[i], nil
		}
	}
	return nil, fmt.Errorf("quarantine record not found for %s", quarantinePath)
}
