package repositories

import (
	"context"

	"tierify/internal/models"
)

// PaginatedResult wraps a slice of results with pagination metadata.
type PaginatedResult[T any] struct {
	Data       []T `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Total  int `json:"total"`
}

type TierRepository interface {
	Create(ctx context.Context, tier *models.Tier, limits []models.LimitDefinition) error
	GetByKey(ctx context.Context, key string) (*models.Tier, error)
	GetByKeyAndVersion(ctx context.Context, key string, version int) (*models.Tier, error)
	ListVersions(ctx context.Context, key string, offset, limit int) (*PaginatedResult[models.Tier], error)
	List(ctx context.Context, filters models.TierFilters) (*PaginatedResult[models.Tier], error)
	Deprecate(ctx context.Context, key string, version int) error
}

type LimitDefinitionRepository interface {
	GetByTier(ctx context.Context, tierID string) ([]models.LimitDefinition, error)
	GetByTierAndKey(ctx context.Context, tierID string, limitKey string) (*models.LimitDefinition, error)
}

type TenantRepository interface {
	Create(ctx context.Context, tenant *models.Tenant) error
	GetByID(ctx context.Context, id string) (*models.Tenant, error)
	GetByExternalID(ctx context.Context, externalID string) (*models.Tenant, error)
	GetChildren(ctx context.Context, tenantID string, offset, limit int) (*PaginatedResult[models.Tenant], error)
	GetAncestors(ctx context.Context, tenantID string) ([]models.Tenant, error)
	ListByTierID(ctx context.Context, tierID string) ([]models.Tenant, error)
	Update(ctx context.Context, tenant *models.Tenant) error
	Delete(ctx context.Context, tenantID string, mode models.DeleteMode) error
	RecordTierChange(ctx context.Context, change *models.TenantTierChange) error
	GetTierHistory(ctx context.Context, tenantID string, offset, limit int) (*PaginatedResult[models.TenantTierChange], error)
}

type UsageRepository interface {
	GetUsage(ctx context.Context, tenantID string, limitKey string) (*models.UsageRecord, error)
	GetAllUsage(ctx context.Context, tenantID string) ([]models.UsageRecord, error)
	UpsertUsage(ctx context.Context, record *models.UsageRecord) error
	ResetUsage(ctx context.Context, tenantID string, limitKey string) error
	ResetAllUsage(ctx context.Context, tenantID string) error
}

type TransactionRepository interface {
	Create(ctx context.Context, tx *models.Transaction) error
	GetByID(ctx context.Context, id string) (*models.Transaction, error)
	AddOperation(ctx context.Context, txID string, op models.TransactionOp) error
	AddLimitKeys(ctx context.Context, txID string, keys []string) error
	UpdateStatus(ctx context.Context, txID string, status models.TransactionStatus) error
	GetPendingByTenantAndLimitKey(ctx context.Context, tenantID string, limitKey string) (*models.Transaction, error)
	CleanExpired(ctx context.Context) (int, error)
}

// Repositories aggregates all repository interfaces.
type Repositories struct {
	Tiers            TierRepository
	LimitDefinitions LimitDefinitionRepository
	Tenants          TenantRepository
	Usage            UsageRepository
	Transactions     TransactionRepository
}
