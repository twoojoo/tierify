package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestAbsoluteEvaluator_UnderLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 50}

	result, err := engine.Check(def, usage, 10, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(50), result.CurrentValue)
	assert.Equal(t, int64(100), result.LimitValue)
	assert.Equal(t, int64(50), result.Remaining)
}

func TestAbsoluteEvaluator_AtLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 100}

	result, err := engine.Check(def, usage, 1, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(0), result.Remaining)
}

func TestAbsoluteEvaluator_ExactlyAtLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 99}

	result, err := engine.Check(def, usage, 1, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(1), result.Remaining)
}

func TestAbsoluteEvaluator_OverLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 95}

	result, err := engine.Check(def, usage, 10, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(5), result.Remaining)
}

func TestAbsoluteEvaluator_NilUsage(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}

	result, err := engine.Check(def, nil, 1, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(0), result.CurrentValue)
	assert.Equal(t, int64(100), result.Remaining)
}

func TestAbsoluteEvaluator_ZeroAmount(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 100}

	result, err := engine.Check(def, usage, 0, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestAbsoluteEvaluator_LargeAmount(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 0}

	result, err := engine.Check(def, usage, 101, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestAbsoluteEvaluator_ResultFields(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "maxItems",
		Kind:   models.LimitKindAbsolute,
		Config: models.AbsoluteConfig{MaxValue: 100},
	}
	usage := &models.UsageRecord{CurrentValue: 30}

	result, err := engine.Check(def, usage, 5, time.Now())
	assert.NoError(t, err)
	assert.Equal(t, "maxItems", result.LimitKey)
	assert.Equal(t, "absolute", result.LimitKind)
	assert.Nil(t, result.ResetAt)
	assert.Nil(t, result.RetryAfterSecs)
}
