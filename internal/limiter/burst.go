package limiter

import (
	"time"

	"tierify/internal/models"
)

// BurstEvaluator uses a token bucket approach.
// Tokens refill at MaxRate per Window. BurstSize is the bucket capacity.
type BurstEvaluator struct{}

func (e *BurstEvaluator) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, now time.Time) *CheckResult {
	cfg := def.Config.(models.BurstConfig)

	// Calculate tokens available using token bucket.
	tokens := e.availableTokens(cfg, usage, now)
	allowed := amount <= tokens

	result := &CheckResult{
		Allowed:      allowed,
		CurrentValue: cfg.BurstSize - tokens, // "used" tokens
		LimitValue:   cfg.BurstSize,
		Remaining:    max(0, tokens),
	}

	if !allowed && tokens < cfg.BurstSize {
		// Calculate time until enough tokens are available.
		deficit := amount - tokens
		refillRate := float64(cfg.MaxRate) / float64(cfg.WindowDuration().Seconds())
		if refillRate > 0 {
			waitSecs := int64(float64(deficit) / refillRate)
			if waitSecs < 1 {
				waitSecs = 1
			}
			result.RetryAfterSecs = &waitSecs
		}
	}

	return result
}

// availableTokens calculates how many tokens are available right now.
func (e *BurstEvaluator) availableTokens(cfg models.BurstConfig, usage *models.UsageRecord, now time.Time) int64 {
	// If no prior usage, full bucket.
	if usage.WindowStart == nil {
		return cfg.BurstSize
	}

	elapsed := now.Sub(*usage.WindowStart)
	windowSecs := cfg.WindowDuration().Seconds()

	// Tokens refilled since last update.
	refillRate := float64(cfg.MaxRate) / windowSecs
	refilled := int64(elapsed.Seconds() * refillRate)

	// CurrentValue represents tokens consumed at WindowStart time.
	available := cfg.BurstSize - usage.CurrentValue + refilled

	if available > cfg.BurstSize {
		available = cfg.BurstSize
	}
	if available < 0 {
		available = 0
	}

	return available
}
