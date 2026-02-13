package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestCompoundEvaluator_AllPass(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:  "apiCalls",
		Kind: models.LimitKindCompound,
		Config: models.CompoundConfig{
			SubLimits: []models.SubLimitConfig{
				MustMarshalSubConfig(models.LimitKindAbsolute, models.AbsoluteConfig{MaxValue: 1000}),
				MustMarshalSubConfig(models.LimitKindAbsolute, models.AbsoluteConfig{MaxValue: 500}),
			},
		},
	}
	usage := &models.UsageRecord{CurrentValue: 100}

	result, err := engine.Check(def, usage, 10, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	// Should return the most restrictive (500 limit, remaining = 400).
	assert.Equal(t, int64(500), result.LimitValue)
	assert.Equal(t, int64(400), result.Remaining)
}

func TestCompoundEvaluator_OneFails(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:  "apiCalls",
		Kind: models.LimitKindCompound,
		Config: models.CompoundConfig{
			SubLimits: []models.SubLimitConfig{
				MustMarshalSubConfig(models.LimitKindAbsolute, models.AbsoluteConfig{MaxValue: 1000}),
				MustMarshalSubConfig(models.LimitKindAbsolute, models.AbsoluteConfig{MaxValue: 100}),
			},
		},
	}
	usage := &models.UsageRecord{CurrentValue: 95}

	result, err := engine.Check(def, usage, 10, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed, "compound should fail if any sub-limit fails")
}

func TestCompoundEvaluator_MixedKinds(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	windowEnd := now.Add(time.Hour)

	def := &models.LimitDefinition{
		Key:  "apiCalls",
		Kind: models.LimitKindCompound,
		Config: models.CompoundConfig{
			SubLimits: []models.SubLimitConfig{
				MustMarshalSubConfig(models.LimitKindAbsolute, models.AbsoluteConfig{MaxValue: 10000}),
				MustMarshalSubConfig(models.LimitKindTimeBased, models.TimeBasedConfig{MaxValue: 100, Period: "hour"}),
			},
		},
	}
	usage := &models.UsageRecord{
		CurrentValue: 50,
		WindowStart:  &now,
		WindowEnd:    &windowEnd,
	}

	result, err := engine.Check(def, usage, 10, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
}
