package dynpkg

import (
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
		defaultPluginRate: 1000,
		defaultTypeRate:   500,
		defaultPidRate:    100,
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
