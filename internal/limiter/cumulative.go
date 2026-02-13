package limiter

import (
	"time"

	"tierify/internal/models"
)

type CumulativeEvaluator struct{}

func (e *CumulativeEvaluator) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, _ time.Time) *CheckResult {
	cfg := def.Config.(models.CumulativeConfig)
	current := usage.CurrentValue
	remaining := cfg.MaxValue - current

	return &CheckResult{
		Allowed:      current+amount <= cfg.MaxValue,
		CurrentValue: current,
		LimitValue:   cfg.MaxValue,
		Remaining:    max(0, remaining),
	}
}
