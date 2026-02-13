package limiter

import (
	"time"

	"tierify/internal/models"
)

type AbsoluteEvaluator struct{}

func (e *AbsoluteEvaluator) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, _ time.Time) *CheckResult {
	cfg := def.Config.(models.AbsoluteConfig)
	current := usage.CurrentValue
	remaining := cfg.MaxValue - current

	return &CheckResult{
		Allowed:      current+amount <= cfg.MaxValue,
		CurrentValue: current,
		LimitValue:   cfg.MaxValue,
		Remaining:    max(0, remaining),
	}
}
