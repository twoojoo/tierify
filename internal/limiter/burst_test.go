package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBurstEvaluator_FullBucket(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:  "requests",
		Kind: models.LimitKindBurst,
		Config: models.BurstConfig{
			MaxRate:   100,
			BurstSize: 200,
			Window:    "minute",
		},
	}

	// No prior usage — full bucket.
	result, err := engine.Check(def, nil, 150, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(200), result.Remaining)
}

func TestBurstEvaluator_ExceedsBurst(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:  "requests",
		Kind: models.LimitKindBurst,
		Config: models.BurstConfig{
			MaxRate:   100,
			BurstSize: 200,
			Window:    "minute",
		},
	}

	result, err := engine.Check(def, nil, 201, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestBurstEvaluator_TokensRefill(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	consumedAt := now.Add(-30 * time.Second) // 30 seconds ago

	def := &models.LimitDefinition{
		Key:  "requests",
		Kind: models.LimitKindBurst,
		Config: models.BurstConfig{
			MaxRate:   60, // 60 per minute = 1 per second
			BurstSize: 100,
			Window:    "minute",
		},
	}
	usage := &models.UsageRecord{
		CurrentValue: 100, // All tokens consumed.
		WindowStart:  &consumedAt,
	}

	// After 30 seconds, 30 tokens should have refilled.
	result, err := engine.Check(def, usage, 25, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestBurstEvaluator_NotEnoughRefill(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	consumedAt := now.Add(-5 * time.Second) // 5 seconds ago

	def := &models.LimitDefinition{
		Key:  "requests",
		Kind: models.LimitKindBurst,
		Config: models.BurstConfig{
			MaxRate:   60, // 1 per second
			BurstSize: 100,
			Window:    "minute",
		},
	}
	usage := &models.UsageRecord{
		CurrentValue: 100,
		WindowStart:  &consumedAt,
	}

	// Only 5 tokens refilled, trying to use 10.
	result, err := engine.Check(def, usage, 10, now)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	require.NotNil(t, result.RetryAfterSecs)
	assert.True(t, *result.RetryAfterSecs > 0)
}

func TestBurstEvaluator_BucketCapsCap(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	longAgo := now.Add(-10 * time.Minute)

	def := &models.LimitDefinition{
		Key:  "requests",
		Kind: models.LimitKindBurst,
		Config: models.BurstConfig{
			MaxRate:   60,
			BurstSize: 100,
			Window:    "minute",
		},
	}
	usage := &models.UsageRecord{
		CurrentValue: 50,
		WindowStart:  &longAgo,
	}

	// Long time passed — bucket should be full (capped at BurstSize).
	result, err := engine.Check(def, usage, 100, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(100), result.Remaining)
}
