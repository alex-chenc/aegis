package dynpkg

import (
	"crypto/ed25519"
	"sync"
)

type Manager struct {
	mu               sync.RWMutex
	storagePath      string
	publicKey        ed25519.PublicKey
	allowlistChecker *AllowlistChecker
	storage          *PackageStorage
	rateLimiter      *RateLimiter
	packages         map[string]*InstalledPackage
	onStatusChange   func(packageID, version, status string)
	onAlert          func(alert interface{})
	sigmaMatcher     SigmaMatcher
	corrEngine       CorrelationEngine
}

type SigmaMatcher interface {
	Match(event map[string]interface{}) []SigmaMatch
	AddRules(rules []byte) error
	RemoveRules(ruleIDs []string)
}

type SigmaMatch struct {
	RuleID    string
	Title     string
	Severity  string
	MitreID   string
	EventType string
}

type CorrelationEngine interface {
	AddSpec(spec interface{}) error
	RemovePackage(packageID string)
	AddFinding(finding interface{}) []interface{}
}

type InstalledPackage struct {
	PackageID               string
	Version                 string
	Manifest                *PackageManifest
	PluginManifest          *PluginManifest
	ActiveArtifact          string
	Status                  string
	LoadedHooks             []string
	ErrorMessage            string
	LoadedSigmaRuleIDs      []string
	LoadedCorrelationFiles  []string
	stateMachine            *StateMachine
}

type DetectionPackageCommand struct {
	CommandID    string `json:"command_id"`
	Action       string `json:"action"`
	PackageID    string `json:"package_id"`
	Version      string `json:"version"`
	PackageURL   string `json:"package_url"`
	SignatureURL string `json:"signature_url"`
	PackageSize  int64  `json:"package_size"`
	Rollback     bool   `json:"rollback"`
}
