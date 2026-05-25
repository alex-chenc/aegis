package dynpkg

import (
	"fmt"
	"sync"
)

type HookAllowlist struct {
	Version     int64    `json:"version"`
	Tracepoints []string `json:"tracepoints"`
	Kprobes     []string `json:"kprobes"`
	LSM         []string `json:"lsm"`
	XDP         []string `json:"xdp"`
	TC          []string `json:"tc"`
}

type AllowlistChecker struct {
	allowlist HookAllowlist
	mu        sync.RWMutex
}

func (c *AllowlistChecker) Update(allowlist HookAllowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowlist = allowlist
}

func (c *AllowlistChecker) Get() HookAllowlist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowlist
}

func (c *AllowlistChecker) IsHookAllowed(hookName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if contains(c.allowlist.Tracepoints, hookName) {
		return true
	}
	if contains(c.allowlist.Kprobes, hookName) {
		return true
	}
	if contains(c.allowlist.LSM, hookName) {
		return true
	}
	if contains(c.allowlist.XDP, hookName) {
		return true
	}
	if contains(c.allowlist.TC, hookName) {
		return true
	}
	return false
}

func (c *AllowlistChecker) CheckPackage(manifest PackageManifest) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if manifest.Plugin.Manifest == "" {
		return nil
	}

	return nil
}

func checkHooksAgainstAllowlist(hooks []PluginHook, allowlist *HookAllowlist) error {
	for _, hook := range hooks {
		allowed := false
		switch hook.AttachType {
		case "tracepoint":
			allowed = contains(allowlist.Tracepoints, hook.Attach)
		case "kprobe":
			allowed = contains(allowlist.Kprobes, hook.Attach)
		case "lsm":
			allowed = contains(allowlist.LSM, hook.Attach)
		case "xdp":
			allowed = contains(allowlist.XDP, hook.Attach)
		case "tc":
			allowed = contains(allowlist.TC, hook.Attach)
		}
		if !allowed {
			return fmt.Errorf("hook %s (%s) not in allowlist", hook.Name, hook.Attach)
		}
	}
	return nil
}
