package models

import "time"

type UsageRecord struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	LimitKey     string     `json:"limit_key"`
	CurrentValue int64      `json:"current_value"`
	WindowStart  *time.Time `json:"window_start,omitempty"`
	WindowEnd    *time.Time `json:"window_end,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
