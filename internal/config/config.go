package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// Server
	Port         int           `env:"PORT" envDefault:"8080"`
	Host         string        `env:"HOST" envDefault:"0.0.0.0"`
	ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"15s"`

	// Database
	DBType          string `env:"DB_TYPE" envDefault:"sqlite"`
	DBConnectionStr string `env:"DB_CONNECTION_STRING" envDefault:"tierify.db"`

	// Behavior
	AllowUsageBelowZero bool `env:"ALLOW_USAGE_BELOW_ZERO" envDefault:"false"`

	// Logging
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config from env: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535, got %d", c.Port)
	}
	switch c.DBType {
	case "sqlite", "postgres", "mongo":
	default:
		return fmt.Errorf("DB_TYPE must be one of: sqlite, postgres, mongo; got %q", c.DBType)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error; got %q", c.LogLevel)
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("LOG_FORMAT must be one of: json, text; got %q", c.LogFormat)
	}
	return nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
