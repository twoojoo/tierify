package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestCumulativeEvaluator_UnderLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "storage",
		Kind:   models.LimitKindCumulative,
		Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"},
	}
	usage := &models.UsageRecord{CurrentValue: 500}

	result, err := engine.Check(def, usage, 100, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(500), result.CurrentValue)
	assert.Equal(t, int64(500), result.Remaining)
}

func TestCumulativeEvaluator_AtLimit(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "storage",
		Kind:   models.LimitKindCumulative,
		Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"},
	}
	usage := &models.UsageRecord{CurrentValue: 1000}

	result, err := engine.Check(def, usage, 1, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(0), result.Remaining)
	assert.Nil(t, result.ResetAt, "cumulative limits never reset")
}

func TestCumulativeEvaluator_NilUsage(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "storage",
		Kind:   models.LimitKindCumulative,
		Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"},
	}

	result, err := engine.Check(def, nil, 100, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(1000), result.Remaining)
}
