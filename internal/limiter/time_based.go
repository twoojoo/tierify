package limiter

import (
	"time"

	"tierify/internal/models"
)

type TimeBasedEvaluator struct{}

func (e *TimeBasedEvaluator) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, now time.Time) *CheckResult {
	cfg := def.Config.(models.TimeBasedConfig)
	current := usage.CurrentValue

	// Check if the window has expired — if so, implicit reset.
	if usage.WindowEnd != nil && now.After(*usage.WindowEnd) {
		current = 0
	}

	remaining := cfg.MaxValue - current
	allowed := current+amount <= cfg.MaxValue

	result := &CheckResult{
		Allowed:      allowed,
		CurrentValue: current,
		LimitValue:   cfg.MaxValue,
		Remaining:    max(0, remaining),
	}

	// Compute reset time.
	windowEnd := e.computeWindowEnd(usage, cfg, now)
	result.ResetAt = &windowEnd

	if !allowed {
		retryAfter := int64(time.Until(windowEnd).Seconds())
		if retryAfter < 0 {
			retryAfter = 0
		}
		result.RetryAfterSecs = &retryAfter
	}

	return result
}

// computeWindowEnd returns the end of the current active window.
func (e *TimeBasedEvaluator) computeWindowEnd(usage *models.UsageRecord, cfg models.TimeBasedConfig, now time.Time) time.Time {
	period := cfg.PeriodDuration()

	// If there's an existing valid window, use it.
	if usage.WindowEnd != nil && now.Before(*usage.WindowEnd) {
		return *usage.WindowEnd
	}

	// Otherwise, new window starts now.
	return now.Add(period)
}

// WindowForUsage computes the window start/end for a usage record being created or reset.
func WindowForUsage(cfg models.TimeBasedConfig, now time.Time) (start, end time.Time) {
	period := cfg.PeriodDuration()
	return now, now.Add(period)
}
