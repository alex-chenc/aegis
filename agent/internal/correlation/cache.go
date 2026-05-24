package correlation

import (
	"sync"
)

type FindingCache struct {
	mu       sync.RWMutex
	findings []AtomicFinding
	maxSize  int
}

func NewFindingCache(maxSize int) *FindingCache {
	return &FindingCache{
		findings: make([]AtomicFinding, 0, maxSize),
		maxSize:  maxSize,
	}
}

func (c *FindingCache) Add(finding AtomicFinding) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.findings) >= c.maxSize {
		c.findings = c.findings[1:]
	}

	c.findings = append(c.findings, finding)
}

func (c *FindingCache) ForEach(fn func(AtomicFinding) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, f := range c.findings {
		if !fn(f) {
			break
		}
	}
}

func (c *FindingCache) RemoveByPackage(packageID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filtered := make([]AtomicFinding, 0, len(c.findings))
	for _, f := range c.findings {
		if f.PackageID != packageID {
			filtered = append(filtered, f)
		}
	}
	c.findings = filtered
}

func (c *FindingCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.findings)
}
