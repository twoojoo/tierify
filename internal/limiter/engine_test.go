package limiter

import (
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestEngine_UnknownKind(t *testing.T) {
	engine := NewEngine()
	def := &models.LimitDefinition{
		Key:    "test",
		Kind:   models.LimitKind("unknown_kind"),
		Config: models.AbsoluteConfig{MaxValue: 100},
	}

	_, err := engine.Check(def, nil, 1, time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_kind")
}

func TestEngine_AllKindsRegistered(t *testing.T) {
	engine := NewEngine()
	kinds := []models.LimitKind{
		models.LimitKindAbsolute,
		models.LimitKindTimeBased,
		models.LimitKindCumulative,
		models.LimitKindBurst,
		models.LimitKindCompound,
		models.LimitKindFeature,
	}

	for _, kind := range kinds {
		_, ok := engine.evaluators[kind]
		assert.True(t, ok, "evaluator should be registered for kind %q", kind)
	}
}

func TestEngine_NilUsageHandled(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name string
		def  *models.LimitDefinition
	}{
		{
			name: "absolute",
			def: &models.LimitDefinition{
				Key: "test", Kind: models.LimitKindAbsolute,
				Config: models.AbsoluteConfig{MaxValue: 100},
			},
		},
		{
			name: "time_based",
			def: &models.LimitDefinition{
				Key: "test", Kind: models.LimitKindTimeBased,
				Config: models.TimeBasedConfig{MaxValue: 100, Period: "hour"},
			},
		},
		{
			name: "cumulative",
			def: &models.LimitDefinition{
				Key: "test", Kind: models.LimitKindCumulative,
				Config: models.CumulativeConfig{MaxValue: 100, Unit: "bytes"},
			},
		},
		{
			name: "feature_enabled",
			def: &models.LimitDefinition{
				Key: "test", Kind: models.LimitKindFeature,
				Config: models.FeatureConfig{Enabled: true},
			},
		},
		{
			name: "feature_disabled",
			def: &models.LimitDefinition{
				Key: "test", Kind: models.LimitKindFeature,
				Config: models.FeatureConfig{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Check(tt.def, nil, 1, time.Now())
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
