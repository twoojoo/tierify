package models

import (
	"encoding/json"
	"fmt"
	"time"
)

type LimitKind string

const (
	LimitKindAbsolute   LimitKind = "absolute"
	LimitKindTimeBased  LimitKind = "time_based"
	LimitKindCumulative LimitKind = "cumulative"
	LimitKindBurst      LimitKind = "burst"
	LimitKindCompound   LimitKind = "compound"
	LimitKindFeature    LimitKind = "feature"
)

type LimitScope string

const (
	LimitScopeGlobal LimitScope = "global"
	LimitScopeLocal  LimitScope = "local"
)

type LimitDefinition struct {
	ID        string      `json:"id"`
	TierID    string      `json:"tier_id"`
	Key       string      `json:"key"`
	Kind      LimitKind   `json:"kind"`
	Scope     LimitScope  `json:"scope"`
	Config    LimitConfig `json:"config"`
	CreatedAt time.Time   `json:"created_at"`
}

// LimitConfig is the interface for kind-specific configurations.
type LimitConfig interface {
	Kind() LimitKind
	Validate() error
}

type AbsoluteConfig struct {
	MaxValue int64 `json:"max_value"`
}

func (c AbsoluteConfig) Kind() LimitKind { return LimitKindAbsolute }
func (c AbsoluteConfig) Validate() error {
	if c.MaxValue <= 0 {
		return fmt.Errorf("absolute max_value must be positive, got %d", c.MaxValue)
	}
	return nil
}

type TimeBasedConfig struct {
	MaxValue int64  `json:"max_value"`
	Period   string `json:"period"` // "minute", "hour", "day", "week", "month"
}

func (c TimeBasedConfig) Kind() LimitKind { return LimitKindTimeBased }
func (c TimeBasedConfig) Validate() error {
	if c.MaxValue <= 0 {
		return fmt.Errorf("time_based max_value must be positive, got %d", c.MaxValue)
	}
	switch c.Period {
	case "minute", "hour", "day", "week", "month":
		return nil
	default:
		return fmt.Errorf("time_based period must be one of: minute, hour, day, week, month; got %q", c.Period)
	}
}

// PeriodDuration returns the duration of the configured period.
func (c TimeBasedConfig) PeriodDuration() time.Duration {
	switch c.Period {
	case "minute":
		return time.Minute
	case "hour":
		return time.Hour
	case "day":
		return 24 * time.Hour
	case "week":
		return 7 * 24 * time.Hour
	case "month":
		return 30 * 24 * time.Hour // Approximation
	default:
		return 0
	}
}

type CumulativeConfig struct {
	MaxValue int64  `json:"max_value"`
	Unit     string `json:"unit"` // Human-readable (e.g., "bytes", "records")
}

func (c CumulativeConfig) Kind() LimitKind { return LimitKindCumulative }
func (c CumulativeConfig) Validate() error {
	if c.MaxValue <= 0 {
		return fmt.Errorf("cumulative max_value must be positive, got %d", c.MaxValue)
	}
	return nil
}

type BurstConfig struct {
	MaxRate   int64  `json:"max_rate"`
	BurstSize int64  `json:"burst_size"`
	Window    string `json:"window"` // "second", "minute", "hour"
}

func (c BurstConfig) Kind() LimitKind { return LimitKindBurst }
func (c BurstConfig) Validate() error {
	if c.MaxRate <= 0 {
		return fmt.Errorf("burst max_rate must be positive, got %d", c.MaxRate)
	}
	if c.BurstSize <= 0 {
		return fmt.Errorf("burst burst_size must be positive, got %d", c.BurstSize)
	}
	if c.BurstSize < c.MaxRate {
		return fmt.Errorf("burst burst_size (%d) must be >= max_rate (%d)", c.BurstSize, c.MaxRate)
	}
	switch c.Window {
	case "second", "minute", "hour":
		return nil
	default:
		return fmt.Errorf("burst window must be one of: second, minute, hour; got %q", c.Window)
	}
}

// WindowDuration returns the duration of the configured window.
func (c BurstConfig) WindowDuration() time.Duration {
	switch c.Window {
	case "second":
		return time.Second
	case "minute":
		return time.Minute
	case "hour":
		return time.Hour
	default:
		return 0
	}
}

type CompoundConfig struct {
	SubLimits []SubLimitConfig `json:"sub_limits"`
}

type SubLimitConfig struct {
	Kind   LimitKind       `json:"kind"`
	Config json.RawMessage `json:"config"`
}

func (c CompoundConfig) Kind() LimitKind { return LimitKindCompound }
func (c CompoundConfig) Validate() error {
	if len(c.SubLimits) < 2 {
		return fmt.Errorf("compound limit must have at least 2 sub-limits, got %d", len(c.SubLimits))
	}
	for i, sub := range c.SubLimits {
		parsed, err := ParseLimitConfig(sub.Kind, sub.Config)
		if err != nil {
			return fmt.Errorf("sub_limit[%d]: %w", i, err)
		}
		if err := parsed.Validate(); err != nil {
			return fmt.Errorf("sub_limit[%d]: %w", i, err)
		}
	}
	return nil
}

type FeatureConfig struct {
	Enabled bool `json:"enabled"`
}

func (c FeatureConfig) Kind() LimitKind { return LimitKindFeature }
func (c FeatureConfig) Validate() error { return nil }

// ParseLimitConfig deserializes a LimitConfig from its kind and raw JSON.
func ParseLimitConfig(kind LimitKind, raw json.RawMessage) (LimitConfig, error) {
	switch kind {
	case LimitKindAbsolute:
		var cfg AbsoluteConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing absolute config: %w", err)
		}
		return cfg, nil
	case LimitKindTimeBased:
		var cfg TimeBasedConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing time_based config: %w", err)
		}
		return cfg, nil
	case LimitKindCumulative:
		var cfg CumulativeConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing cumulative config: %w", err)
		}
		return cfg, nil
	case LimitKindBurst:
		var cfg BurstConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing burst config: %w", err)
		}
		return cfg, nil
	case LimitKindCompound:
		var cfg CompoundConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing compound config: %w", err)
		}
		return cfg, nil
	case LimitKindFeature:
		var cfg FeatureConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parsing feature config: %w", err)
		}
		return cfg, nil
	default:
		return nil, fmt.Errorf("unknown limit kind: %q", kind)
	}
}

// MarshalLimitConfig serializes a LimitConfig to JSON.
func MarshalLimitConfig(cfg LimitConfig) (json.RawMessage, error) {
	return json.Marshal(cfg)
}
