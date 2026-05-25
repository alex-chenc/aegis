package dynpkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type PackageStateRecord struct {
	PackageID string       `json:"package_id"`
	Version   string       `json:"version"`
	State     PackageState `json:"state"`
	UpdatedAt time.Time    `json:"updated_at"`
	Reason    string       `json:"reason,omitempty"`
}

type PackageStorage struct {
	baseDir string
}

func NewPackageStorage(baseDir string) *PackageStorage {
	return &PackageStorage{baseDir: baseDir}
}

func (s *PackageStorage) SaveState(record PackageStateRecord) error {
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.json", record.PackageID, record.Version)
	path := filepath.Join(s.baseDir, filename)

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state record: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

func (s *PackageStorage) LoadState(packageID, version string) (*PackageStateRecord, error) {
	filename := fmt.Sprintf("%s_%s.json", packageID, version)
	path := filepath.Join(s.baseDir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var record PackageStateRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("unmarshal state record: %w", err)
	}
	return &record, nil
}

func (s *PackageStorage) ListInstalled() ([]PackageStateRecord, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	var records []PackageStateRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.baseDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var record PackageStateRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}
