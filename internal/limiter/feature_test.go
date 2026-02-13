package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestFeatureEvaluator_Enabled(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "advancedReports",
		Kind:   models.LimitKindFeature,
		Config: models.FeatureConfig{Enabled: true},
	}

	result, err := engine.Check(def, nil, 0, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "advancedReports", result.LimitKey)
	assert.Equal(t, "feature", result.LimitKind)
}

func TestFeatureEvaluator_Disabled(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "advancedReports",
		Kind:   models.LimitKindFeature,
		Config: models.FeatureConfig{Enabled: false},
	}

	result, err := engine.Check(def, nil, 0, time.Now())
	assert.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestFeatureEvaluator_IgnoresUsageAndAmount(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "advancedReports",
		Kind:   models.LimitKindFeature,
		Config: models.FeatureConfig{Enabled: true},
	}
	usage := &models.UsageRecord{CurrentValue: 999}

	result, err := engine.Check(def, usage, 1000, time.Now())
	assert.NoError(t, err)
	assert.True(t, result.Allowed, "feature limits ignore usage and amount")
}
