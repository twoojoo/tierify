package models

import "time"

type Tier struct {
	ID         string    `json:"id"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	Version    int       `json:"version"`
	Deprecated bool      `json:"deprecated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TierUpdateOptions struct {
	MigrateExisting    bool   `json:"migrate_existing"`
	DeprecatePrevious  string `json:"deprecate_previous"` // "none", "latest", "all"
	ResetUsage         bool   `json:"reset_usage"`
}

type TierFilters struct {
	Key        *string
	Deprecated *bool
	Offset     int
	Limit      int
}
