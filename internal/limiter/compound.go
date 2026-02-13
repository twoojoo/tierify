package limiter

import (
	"encoding/json"
	"time"

	"tierify/internal/models"
)

// CompoundEvaluator checks all sub-limits. All must pass (AND logic).
// Returns the most restrictive result.
type CompoundEvaluator struct {
	engine *Engine
}

func (e *CompoundEvaluator) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, now time.Time) *CheckResult {
	cfg := def.Config.(models.CompoundConfig)

	var mostRestrictive *CheckResult

	for _, sub := range cfg.SubLimits {
		subConfig, err := models.ParseLimitConfig(sub.Kind, sub.Config)
		if err != nil {
			return &CheckResult{Allowed: false, LimitKey: def.Key, LimitKind: string(def.Kind)}
		}

		subDef := &models.LimitDefinition{
			Key:    def.Key,
			Kind:   sub.Kind,
			Config: subConfig,
		}

		ev, ok := e.engine.evaluators[sub.Kind]
		if !ok {
			return &CheckResult{Allowed: false, LimitKey: def.Key, LimitKind: string(def.Kind)}
		}

		result := ev.Check(subDef, usage, amount, now)

		if mostRestrictive == nil || result.Remaining < mostRestrictive.Remaining {
			mostRestrictive = result
		}

		if !result.Allowed {
			// Return immediately on first failure — compound is AND.
			return &CheckResult{
				Allowed:        false,
				CurrentValue:   result.CurrentValue,
				LimitValue:     result.LimitValue,
				Remaining:      result.Remaining,
				ResetAt:        result.ResetAt,
				RetryAfterSecs: result.RetryAfterSecs,
			}
		}
	}

	if mostRestrictive == nil {
		return &CheckResult{Allowed: true}
	}

	return mostRestrictive
}

// MustMarshalSubConfig is a helper to create SubLimitConfig from typed configs.
func MustMarshalSubConfig(kind models.LimitKind, cfg models.LimitConfig) models.SubLimitConfig {
	raw, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return models.SubLimitConfig{
		Kind:   kind,
		Config: raw,
	}
}
