package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeBasedEvaluator_UnderLimit(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	windowEnd := now.Add(time.Hour)

	def := &models.LimitDefinition{
		Key:    "apiCalls",
		Kind:   models.LimitKindTimeBased,
		Config: models.TimeBasedConfig{MaxValue: 100, Period: "hour"},
	}
	usage := &models.UsageRecord{
		CurrentValue: 50,
		WindowStart:  &now,
		WindowEnd:    &windowEnd,
	}

	result, err := engine.Check(def, usage, 10, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(50), result.CurrentValue)
	assert.Equal(t, int64(50), result.Remaining)
	assert.NotNil(t, result.ResetAt)
}

func TestTimeBasedEvaluator_WindowExpired(t *testing.T) {
	engine := NewEngine()
	past := time.Now().Add(-2 * time.Hour)
	pastEnd := past.Add(time.Hour)
	now := time.Now()

	def := &models.LimitDefinition{
		Key:    "apiCalls",
		Kind:   models.LimitKindTimeBased,
		Config: models.TimeBasedConfig{MaxValue: 100, Period: "hour"},
	}
	usage := &models.UsageRecord{
		CurrentValue: 100, // Was at limit, but window expired.
		WindowStart:  &past,
		WindowEnd:    &pastEnd,
	}

	result, err := engine.Check(def, usage, 10, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed, "should be allowed after window expired")
	assert.Equal(t, int64(0), result.CurrentValue, "current value should reset to 0")
	assert.Equal(t, int64(100), result.Remaining)
}

func TestTimeBasedEvaluator_AtLimitWithinWindow(t *testing.T) {
	engine := NewEngine()
	now := time.Now()
	windowEnd := now.Add(30 * time.Minute)

	def := &models.LimitDefinition{
		Key:    "apiCalls",
		Kind:   models.LimitKindTimeBased,
		Config: models.TimeBasedConfig{MaxValue: 100, Period: "hour"},
	}
	usage := &models.UsageRecord{
		CurrentValue: 100,
		WindowStart:  &now,
		WindowEnd:    &windowEnd,
	}

	result, err := engine.Check(def, usage, 1, now)
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(0), result.Remaining)
	require.NotNil(t, result.RetryAfterSecs)
	assert.True(t, *result.RetryAfterSecs > 0)
}

func TestTimeBasedEvaluator_NilUsage(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	def := &models.LimitDefinition{
		Key:    "apiCalls",
		Kind:   models.LimitKindTimeBased,
		Config: models.TimeBasedConfig{MaxValue: 100, Period: "hour"},
	}

	result, err := engine.Check(def, nil, 1, now)
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(0), result.CurrentValue)
	assert.Equal(t, int64(100), result.Remaining)
	assert.NotNil(t, result.ResetAt)
}

func TestTimeBasedEvaluator_DifferentPeriods(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	tests := []struct {
		period   string
		duration time.Duration
	}{
		{"minute", time.Minute},
		{"hour", time.Hour},
		{"day", 24 * time.Hour},
		{"week", 7 * 24 * time.Hour},
		{"month", 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			def := &models.LimitDefinition{
				Key:    "calls",
				Kind:   models.LimitKindTimeBased,
				Config: models.TimeBasedConfig{MaxValue: 10, Period: tt.period},
			}

			result, err := engine.Check(def, nil, 1, now)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			require.NotNil(t, result.ResetAt)
			assert.Equal(t, now.Add(tt.duration), *result.ResetAt)
		})
	}
}

func TestWindowForUsage(t *testing.T) {
	now := time.Now()
	cfg := models.TimeBasedConfig{MaxValue: 100, Period: "hour"}

	start, end := WindowForUsage(cfg, now)
	assert.Equal(t, now, start)
	assert.Equal(t, now.Add(time.Hour), end)
}
