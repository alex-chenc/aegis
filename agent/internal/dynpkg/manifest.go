package dynpkg

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type PackageManifest struct {
	SchemaVersion    string        `yaml:"schema_version"`
	PackageID        string        `yaml:"package_id"`
	Version          string        `yaml:"version"`
	Title            string        `yaml:"title"`
	Description      string        `yaml:"description"`
	MinAgentVersion  string        `yaml:"min_agent_version"`
	Plugin           PluginRef     `yaml:"plugin"`
	Artifacts        ArtifactRefs  `yaml:"artifacts"`
	SigmaRules       []string      `yaml:"sigma_rules"`
	CorrelationRules []string      `yaml:"correlation_rules"`
	Limits           PackageLimits `yaml:"limits"`
}

type PluginManifest struct {
	SchemaVersion string            `yaml:"schema_version"`
	PluginID      string            `yaml:"plugin_id"`
	PackageID     string            `yaml:"package_id"`
	EventMap      string            `yaml:"event_map"`
	Hooks         []PluginHook      `yaml:"hooks"`
	EventSchema   PluginEventSchema `yaml:"event_schema"`
}

type PluginHook struct {
	Name       string `yaml:"name"`
	AttachType string `yaml:"attach_type"`
	Attach     string `yaml:"attach"`
	Program    string `yaml:"program"`
}

type PluginRef struct {
	Manifest string `yaml:"manifest"`
}

type ArtifactRefs struct {
	Perf    string `yaml:"perf"`
	Ringbuf string `yaml:"ringbuf"`
}

type PackageArtifact struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
	Hash string `yaml:"hash"`
}

type PackageLimits struct {
	MaxEventsPerSecond             int  `yaml:"max_events_per_second"`
	MaxEventsPerPidPerSecond       int  `yaml:"max_events_per_pid_per_second"`
	AutoDisableOnSustainedOverflow bool `yaml:"auto_disable_on_sustained_overflow"`
}

type PluginEventSchema struct {
	Events map[int]EventDef `yaml:"events"`
}

type EventDef struct {
	Name   string           `yaml:"name"`
	Fields map[int]FieldDef `yaml:"fields"`
}

type FieldDef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

func ParseManifest(data []byte) (*PackageManifest, error) {
	var manifest PackageManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse package manifest: %w", err)
	}
	return &manifest, nil
}

func ValidateManifest(m *PackageManifest) error {
	if m.PackageID == "" {
		return fmt.Errorf("package_id is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if m.Plugin.Manifest == "" {
		return fmt.Errorf("plugin.manifest is required")
	}
	return nil
}

func ParseManifestFromFile(path string) (*PackageManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest file: %w", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}
	if err := ValidateManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}

func ParsePluginManifestFromFile(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read plugin manifest file: %w", err)
	}
	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse plugin manifest: %w", err)
	}
	return &manifest, nil
}
