package services

import (
	"context"
	"fmt"

	"tierify/internal/models"
	"tierify/internal/repositories"
)

// --- Mock TierRepository ---

type mockTierRepo struct {
	tiers  map[string]*models.Tier // keyed by "key:version"
	limits map[string][]models.LimitDefinition // keyed by tier ID
}

func newMockTierRepo() *mockTierRepo {
	return &mockTierRepo{
		tiers:  make(map[string]*models.Tier),
		limits: make(map[string][]models.LimitDefinition),
	}
}

func tierKey(key string, version int) string {
	return key + ":" + itoa(version)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func (m *mockTierRepo) Create(_ context.Context, tier *models.Tier, limits []models.LimitDefinition) error {
	m.tiers[tierKey(tier.Key, tier.Version)] = tier
	m.limits[tier.ID] = limits
	return nil
}

func (m *mockTierRepo) GetByKey(_ context.Context, key string) (*models.Tier, error) {
	var latest *models.Tier
	for _, t := range m.tiers {
		if t.Key == key && !t.Deprecated {
			if latest == nil || t.Version > latest.Version {
				latest = t
			}
		}
	}
	return latest, nil
}

func (m *mockTierRepo) GetByKeyAndVersion(_ context.Context, key string, version int) (*models.Tier, error) {
	return m.tiers[tierKey(key, version)], nil
}

func (m *mockTierRepo) ListVersions(_ context.Context, key string, offset, limit int) (*repositories.PaginatedResult[models.Tier], error) {
	var results []models.Tier
	for _, t := range m.tiers {
		if t.Key == key {
			results = append(results, *t)
		}
	}
	total := len(results)
	if offset >= len(results) {
		results = nil
	} else {
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	}
	return &repositories.PaginatedResult[models.Tier]{
		Data:       results,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

func (m *mockTierRepo) List(_ context.Context, filters models.TierFilters) (*repositories.PaginatedResult[models.Tier], error) {
	var results []models.Tier
	for _, t := range m.tiers {
		if filters.Key != nil && t.Key != *filters.Key {
			continue
		}
		if filters.Deprecated != nil && t.Deprecated != *filters.Deprecated {
			continue
		}
		results = append(results, *t)
	}
	total := len(results)
	offset := filters.Offset
	limit := filters.Limit
	if offset >= len(results) {
		results = nil
	} else {
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	}
	return &repositories.PaginatedResult[models.Tier]{
		Data:       results,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

func (m *mockTierRepo) Deprecate(_ context.Context, key string, version int) error {
	k := tierKey(key, version)
	if t, ok := m.tiers[k]; ok {
		t.Deprecated = true
	}
	return nil
}

// --- Mock LimitDefinitionRepository ---

type mockLimitDefRepo struct {
	parent *mockTierRepo
}

func (m *mockLimitDefRepo) GetByTier(_ context.Context, tierID string) ([]models.LimitDefinition, error) {
	return m.parent.limits[tierID], nil
}

func (m *mockLimitDefRepo) GetByTierAndKey(_ context.Context, tierID string, limitKey string) (*models.LimitDefinition, error) {
	for i, l := range m.parent.limits[tierID] {
		if l.Key == limitKey {
			return &m.parent.limits[tierID][i], nil
		}
	}
	return nil, nil
}

// --- Mock TenantRepository ---

type mockTenantRepo struct {
	tenants map[string]*models.Tenant
	history map[string][]models.TenantTierChange
}

func newMockTenantRepo() *mockTenantRepo {
	return &mockTenantRepo{
		tenants: make(map[string]*models.Tenant),
		history: make(map[string][]models.TenantTierChange),
	}
}

func (m *mockTenantRepo) Create(_ context.Context, tenant *models.Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockTenantRepo) GetByID(_ context.Context, id string) (*models.Tenant, error) {
	return m.tenants[id], nil
}

func (m *mockTenantRepo) GetByExternalID(_ context.Context, externalID string) (*models.Tenant, error) {
	for _, t := range m.tenants {
		if t.ExternalID == externalID {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockTenantRepo) GetChildren(_ context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.Tenant], error) {
	var results []models.Tenant
	for _, t := range m.tenants {
		if t.ParentID != nil && *t.ParentID == tenantID {
			results = append(results, *t)
		}
	}
	total := len(results)
	if offset >= len(results) {
		results = nil
	} else {
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	}
	return &repositories.PaginatedResult[models.Tenant]{
		Data:       results,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

func (m *mockTenantRepo) GetAncestors(_ context.Context, tenantID string) ([]models.Tenant, error) {
	var ancestors []models.Tenant
	current := m.tenants[tenantID]
	for current != nil && current.ParentID != nil {
		parent := m.tenants[*current.ParentID]
		if parent == nil {
			break
		}
		ancestors = append(ancestors, *parent)
		current = parent
	}
	return ancestors, nil
}

func (m *mockTenantRepo) ListByTierID(_ context.Context, tierID string) ([]models.Tenant, error) {
	var results []models.Tenant
	for _, t := range m.tenants {
		if t.TierID != nil && *t.TierID == tierID {
			results = append(results, *t)
		}
	}
	return results, nil
}

func (m *mockTenantRepo) Update(_ context.Context, tenant *models.Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockTenantRepo) Delete(_ context.Context, tenantID string, mode models.DeleteMode) error {
	if mode == models.DeleteModeCascade {
		// Delete all descendants.
		toDelete := []string{tenantID}
		for len(toDelete) > 0 {
			id := toDelete[0]
			toDelete = toDelete[1:]
			for _, t := range m.tenants {
				if t.ParentID != nil && *t.ParentID == id {
					toDelete = append(toDelete, t.ID)
				}
			}
			delete(m.tenants, id)
		}
	} else {
		// Orphan: nil out parent references.
		for _, t := range m.tenants {
			if t.ParentID != nil && *t.ParentID == tenantID {
				t.ParentID = nil
			}
		}
		delete(m.tenants, tenantID)
	}
	return nil
}

func (m *mockTenantRepo) RecordTierChange(_ context.Context, change *models.TenantTierChange) error {
	m.history[change.TenantID] = append(m.history[change.TenantID], *change)
	return nil
}

func (m *mockTenantRepo) GetTierHistory(_ context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.TenantTierChange], error) {
	h := m.history[tenantID]
	total := len(h)
	if offset >= len(h) {
		h = nil
	} else {
		end := offset + limit
		if end > len(h) {
			end = len(h)
		}
		h = h[offset:end]
	}
	return &repositories.PaginatedResult[models.TenantTierChange]{
		Data:       h,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

// --- Mock UsageRepository ---

type mockUsageRepo struct {
	records map[string]*models.UsageRecord // keyed by "tenantID:limitKey"
}

func newMockUsageRepo() *mockUsageRepo {
	return &mockUsageRepo{records: make(map[string]*models.UsageRecord)}
}

func usageKey(tenantID, limitKey string) string {
	return tenantID + ":" + limitKey
}

func (m *mockUsageRepo) GetUsage(_ context.Context, tenantID string, limitKey string) (*models.UsageRecord, error) {
	return m.records[usageKey(tenantID, limitKey)], nil
}

func (m *mockUsageRepo) GetAllUsage(_ context.Context, tenantID string) ([]models.UsageRecord, error) {
	var results []models.UsageRecord
	for _, r := range m.records {
		if r.TenantID == tenantID {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (m *mockUsageRepo) UpsertUsage(_ context.Context, record *models.UsageRecord) error {
	m.records[usageKey(record.TenantID, record.LimitKey)] = record
	return nil
}

func (m *mockUsageRepo) ResetUsage(_ context.Context, tenantID string, limitKey string) error {
	delete(m.records, usageKey(tenantID, limitKey))
	return nil
}

func (m *mockUsageRepo) ResetAllUsage(_ context.Context, tenantID string) error {
	for key, r := range m.records {
		if r.TenantID == tenantID {
			delete(m.records, key)
		}
	}
	return nil
}

// --- Mock TransactionRepository ---

type mockTransactionRepo struct {
	txns map[string]*models.Transaction
}

func newMockTransactionRepo() *mockTransactionRepo {
	return &mockTransactionRepo{txns: make(map[string]*models.Transaction)}
}

func (m *mockTransactionRepo) Create(_ context.Context, tx *models.Transaction) error {
	m.txns[tx.ID] = tx
	return nil
}

func (m *mockTransactionRepo) GetByID(_ context.Context, id string) (*models.Transaction, error) {
	return m.txns[id], nil
}

func (m *mockTransactionRepo) AddOperation(_ context.Context, txID string, op models.TransactionOp) error {
	if tx, ok := m.txns[txID]; ok {
		tx.Operations = append(tx.Operations, op)
	}
	return nil
}

func (m *mockTransactionRepo) AddLimitKeys(_ context.Context, txID string, keys []string) error {
	if tx, ok := m.txns[txID]; ok {
		tx.LimitKeys = append(tx.LimitKeys, keys...)
	}
	return nil
}

func (m *mockTransactionRepo) UpdateStatus(_ context.Context, txID string, status models.TransactionStatus) error {
	if tx, ok := m.txns[txID]; ok {
		tx.Status = status
	}
	return nil
}

func (m *mockTransactionRepo) GetPendingByTenantAndLimitKey(_ context.Context, tenantID string, limitKey string) (*models.Transaction, error) {
	for _, tx := range m.txns {
		if tx.TenantID == tenantID && tx.Status == models.TransactionStatusPending {
			for _, k := range tx.LimitKeys {
				if k == limitKey {
					return tx, nil
				}
			}
		}
	}
	return nil, nil
}

func (m *mockTransactionRepo) CleanExpired(_ context.Context) (int, error) {
	return 0, nil
}

// --- Helper to build repositories ---

func newMockRepos() (*repositories.Repositories, *mockTierRepo, *mockTenantRepo, *mockUsageRepo, *mockTransactionRepo) {
	tierRepo := newMockTierRepo()
	tenantRepo := newMockTenantRepo()
	usageRepo := newMockUsageRepo()
	txRepo := newMockTransactionRepo()

	return &repositories.Repositories{
		Tiers:            tierRepo,
		LimitDefinitions: &mockLimitDefRepo{parent: tierRepo},
		Tenants:          tenantRepo,
		Usage:            usageRepo,
		Transactions:     txRepo,
	}, tierRepo, tenantRepo, usageRepo, txRepo
}
