package limiter

import (
	"fmt"
	"time"

	"tierify/internal/models"
)

// CheckResult contains the outcome of evaluating a single limit.
type CheckResult struct {
	Allowed        bool       `json:"available"`
	LimitKey       string     `json:"limit_key"`
	CurrentValue   int64      `json:"current_value"`
	LimitValue     int64      `json:"limit_value"`
	Remaining      int64      `json:"remaining"`
	LimitKind      string     `json:"limit_kind"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
	RetryAfterSecs *int64     `json:"retry_after_seconds,omitempty"`
}

// Evaluator checks whether an operation is allowed for a specific limit kind.
type Evaluator interface {
	Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, now time.Time) *CheckResult
}

// Engine dispatches limit checks to the appropriate evaluator.
type Engine struct {
	evaluators map[models.LimitKind]Evaluator
}

func NewEngine() *Engine {
	e := &Engine{
		evaluators: make(map[models.LimitKind]Evaluator),
	}
	e.evaluators[models.LimitKindAbsolute] = &AbsoluteEvaluator{}
	e.evaluators[models.LimitKindTimeBased] = &TimeBasedEvaluator{}
	e.evaluators[models.LimitKindCumulative] = &CumulativeEvaluator{}
	e.evaluators[models.LimitKindBurst] = &BurstEvaluator{}
	e.evaluators[models.LimitKindCompound] = &CompoundEvaluator{engine: e}
	e.evaluators[models.LimitKindFeature] = &FeatureEvaluator{}
	return e
}

// Check evaluates whether the given amount can be applied against the limit.
// The usage record may be nil (no prior usage), in which case current value is 0.
func (e *Engine) Check(def *models.LimitDefinition, usage *models.UsageRecord, amount int64, now time.Time) (*CheckResult, error) {
	ev, ok := e.evaluators[def.Kind]
	if !ok {
		return nil, fmt.Errorf("no evaluator registered for limit kind %q", def.Kind)
	}
	if usage == nil {
		usage = &models.UsageRecord{
			TenantID:     "",
			LimitKey:     def.Key,
			CurrentValue: 0,
		}
	}
	result := ev.Check(def, usage, amount, now)
	result.LimitKey = def.Key
	result.LimitKind = string(def.Kind)
	return result, nil
}
