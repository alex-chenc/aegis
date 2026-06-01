package dynpkg

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	pluginLimiters    map[string]*rate.Limiter
	typeLimiters      map[string]*rate.Limiter
	pidLimiters       map[int]*rate.Limiter
	mu                sync.Mutex
	disableCallback   func(packageID, reason string)
	exceedStart       map[string]time.Time
	defaultPluginRate rate.Limit
	defaultTypeRate   rate.Limit
	defaultPidRate    rate.Limit
	disableThreshold  time.Duration
}

func NewRateLimiter(disableCallback func(packageID, reason string)) *RateLimiter {
	return &RateLimiter{
		pluginLimiters:    make(map[string]*rate.Limiter),
		typeLimiters:      make(map[string]*rate.Limiter),
		pidLimiters:       make(map[int]*rate.Limiter),
		exceedStart:       make(map[string]time.Time),
		disableCallback:   disableCallback,
		defaultPluginRate: 10000,
		defaultTypeRate:   5000,
		defaultPidRate:    1000,
		disableThreshold:  30 * time.Second,
	}
}

func (rl *RateLimiter) Allow(pluginID, eventType string, pid int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	pluginLimiter, ok := rl.pluginLimiters[pluginID]
	if !ok {
		pluginLimiter = rate.NewLimiter(rl.defaultPluginRate, int(rl.defaultPluginRate))
		rl.pluginLimiters[pluginID] = pluginLimiter
	}

	typeLimiter, ok := rl.typeLimiters[eventType]
	if !ok {
		typeLimiter = rate.NewLimiter(rl.defaultTypeRate, int(rl.defaultTypeRate))
		rl.typeLimiters[eventType] = typeLimiter
	}

	pidLimiter, ok := rl.pidLimiters[pid]
	if !ok {
		pidLimiter = rate.NewLimiter(rl.defaultPidRate, int(rl.defaultPidRate))
		rl.pidLimiters[pid] = pidLimiter
	}

	pluginOk := pluginLimiter.Allow()
	typeOk := typeLimiter.Allow()
	pidOk := pidLimiter.Allow()

	if !pluginOk || !typeOk || !pidOk {
		// Debug: log rate limit hit
		if !pluginOk {
			fmt.Printf("[RateLimit] plugin %s rate exceeded for type=%s pid=%d\n", pluginID, eventType, pid)
		}
		if !typeOk {
			fmt.Printf("[RateLimit] type %s rate exceeded for plugin=%s pid=%d\n", eventType, pluginID, pid)
		}
		if !pidOk {
			fmt.Printf("[RateLimit] pid %d rate exceeded for plugin=%s type=%s\n", pid, pluginID, eventType)
		}
		rl.checkExceed(pluginID)
		return false
	}

	if start, exists := rl.exceedStart[pluginID]; exists {
		_ = start
		delete(rl.exceedStart, pluginID)
	}

	return true
}

func (rl *RateLimiter) UpdateLimits(pluginID string, pluginRate, typeRate, pidRate rate.Limit) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, ok := rl.pluginLimiters[pluginID]; ok {
		limiter.SetLimit(pluginRate)
	} else {
		rl.pluginLimiters[pluginID] = rate.NewLimiter(pluginRate, int(pluginRate))
	}

	if limiter, ok := rl.typeLimiters[pluginID]; ok {
		limiter.SetLimit(typeRate)
	} else {
		rl.typeLimiters[pluginID] = rate.NewLimiter(typeRate, int(typeRate))
	}

	rl.defaultPluginRate = pluginRate
	rl.defaultTypeRate = typeRate
	rl.defaultPidRate = pidRate
}

func (rl *RateLimiter) RemovePackage(pluginID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.pluginLimiters, pluginID)
	delete(rl.exceedStart, pluginID)
}

func (rl *RateLimiter) checkExceed(pluginID string) {
	now := time.Now()
	if start, exists := rl.exceedStart[pluginID]; exists {
		if now.Sub(start) > rl.disableThreshold {
			if rl.disableCallback != nil {
				rl.disableCallback(pluginID, "sustained_rate_exceeded")
			}
			delete(rl.exceedStart, pluginID)
		}
	} else {
		rl.exceedStart[pluginID] = now
	}
}
