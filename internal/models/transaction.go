package models

import "time"

type TransactionStatus string

const (
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusCommitted  TransactionStatus = "committed"
	TransactionStatusRolledBack TransactionStatus = "rolled_back"
	TransactionStatusExpired    TransactionStatus = "expired"
)

type Transaction struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	LimitKeys  []string          `json:"limit_keys"`
	Status     TransactionStatus `json:"status"`
	TTL        *time.Duration    `json:"ttl,omitempty"`
	Operations []TransactionOp   `json:"operations"`
	CreatedAt  time.Time         `json:"created_at"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
}

type TransactionOp struct {
	LimitKey  string `json:"limit_key"`
	Operation string `json:"operation"` // "increment" or "decrement"
	Amount    int64  `json:"amount"`
}
