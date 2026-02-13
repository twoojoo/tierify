package limiter

import (
	"time"

	"tierify/internal/models"
)

type FeatureEvaluator struct{}

func (e *FeatureEvaluator) Check(def *models.LimitDefinition, _ *models.UsageRecord, _ int64, _ time.Time) *CheckResult {
	cfg := def.Config.(models.FeatureConfig)

	var limitValue int64
	if cfg.Enabled {
		limitValue = 1
	}

	return &CheckResult{
		Allowed:      cfg.Enabled,
		CurrentValue: 0,
		LimitValue:   limitValue,
		Remaining:    limitValue,
	}
}
