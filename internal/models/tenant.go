package models

import "time"

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusBlocked   TenantStatus = "blocked"
)

type DeleteMode string

const (
	DeleteModeCascade DeleteMode = "cascade"
	DeleteModeOrphan  DeleteMode = "orphan"
)

type Tenant struct {
	ID          string         `json:"id"`
	ExternalID  string         `json:"external_id"`
	ParentID    *string        `json:"parent_id,omitempty"`
	TierID      *string        `json:"tier_id,omitempty"`
	TierVersion *int           `json:"tier_version,omitempty"`
	Status      TenantStatus   `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type TenantTierChange struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id"`
	PreviousTierID  *string `json:"previous_tier_id,omitempty"`
	PreviousVersion *int    `json:"previous_version,omitempty"`
	NewTierID       *string `json:"new_tier_id,omitempty"`
	NewVersion      *int    `json:"new_version,omitempty"`
	UsageReset      bool    `json:"usage_reset"`
	ChangedAt       time.Time `json:"changed_at"`
}
