package handlers

import (
	"context"
	"fmt"

	"tierify/internal/models"
	"tierify/internal/repositories"
)

// In-memory mock repositories for handler tests.
// These are similar to the service mocks but live in the handlers package.

func newHandlerMockRepos() *repositories.Repositories {
	tierRepo := &hMockTierRepo{
		tiers:  make(map[string]*models.Tier),
		limits: make(map[string][]models.LimitDefinition),
	}
	return &repositories.Repositories{
		Tiers:            tierRepo,
		LimitDefinitions: &hMockLimitDefRepo{parent: tierRepo},
		Tenants:          &hMockTenantRepo{tenants: make(map[string]*models.Tenant), history: make(map[string][]models.TenantTierChange)},
		Usage:            &hMockUsageRepo{records: make(map[string]*models.UsageRecord)},
		Transactions:     &hMockTransactionRepo{txns: make(map[string]*models.Transaction)},
	}
}

// --- Tier ---

type hMockTierRepo struct {
	tiers  map[string]*models.Tier
	limits map[string][]models.LimitDefinition
}

func hTierKey(key string, version int) string { return fmt.Sprintf("%s:%d", key, version) }

func (m *hMockTierRepo) Create(_ context.Context, tier *models.Tier, limits []models.LimitDefinition) error {
	m.tiers[hTierKey(tier.Key, tier.Version)] = tier
	m.limits[tier.ID] = limits
	return nil
}
func (m *hMockTierRepo) GetByKey(_ context.Context, key string) (*models.Tier, error) {
	var latest *models.Tier
	for _, t := range m.tiers {
		if t.Key == key && !t.Deprecated && (latest == nil || t.Version > latest.Version) {
			latest = t
		}
	}
	return latest, nil
}
func (m *hMockTierRepo) GetByKeyAndVersion(_ context.Context, key string, version int) (*models.Tier, error) {
	return m.tiers[hTierKey(key, version)], nil
}
func (m *hMockTierRepo) ListVersions(_ context.Context, key string, offset, limit int) (*repositories.PaginatedResult[models.Tier], error) {
	var results []models.Tier
	for _, t := range m.tiers {
		if t.Key == key {
			results = append(results, *t)
		}
	}
	total := len(results)
	end := offset + limit
	if end > total {
		end = total
	}
	if offset >= total {
		results = nil
	} else {
		results = results[offset:end]
	}
	return &repositories.PaginatedResult[models.Tier]{Data: results, Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total}}, nil
}
func (m *hMockTierRepo) List(_ context.Context, filters models.TierFilters) (*repositories.PaginatedResult[models.Tier], error) {
	var results []models.Tier
	for _, t := range m.tiers {
		results = append(results, *t)
	}
	return &repositories.PaginatedResult[models.Tier]{Data: results, Pagination: repositories.Pagination{Offset: filters.Offset, Limit: filters.Limit, Total: len(results)}}, nil
}
func (m *hMockTierRepo) Deprecate(_ context.Context, key string, version int) error {
	if t, ok := m.tiers[hTierKey(key, version)]; ok {
		t.Deprecated = true
	}
	return nil
}

type hMockLimitDefRepo struct{ parent *hMockTierRepo }

func (m *hMockLimitDefRepo) GetByTier(_ context.Context, tierID string) ([]models.LimitDefinition, error) {
	return m.parent.limits[tierID], nil
}
func (m *hMockLimitDefRepo) GetByTierAndKey(_ context.Context, tierID, limitKey string) (*models.LimitDefinition, error) {
	for i, l := range m.parent.limits[tierID] {
		if l.Key == limitKey {
			return &m.parent.limits[tierID][i], nil
		}
	}
	return nil, nil
}

// --- Tenant ---

type hMockTenantRepo struct {
	tenants map[string]*models.Tenant
	history map[string][]models.TenantTierChange
}

func (m *hMockTenantRepo) Create(_ context.Context, t *models.Tenant) error {
	m.tenants[t.ID] = t; return nil
}
func (m *hMockTenantRepo) GetByID(_ context.Context, id string) (*models.Tenant, error) {
	return m.tenants[id], nil
}
func (m *hMockTenantRepo) GetByExternalID(_ context.Context, eid string) (*models.Tenant, error) {
	for _, t := range m.tenants {
		if t.ExternalID == eid {
			return t, nil
		}
	}
	return nil, nil
}
func (m *hMockTenantRepo) GetChildren(_ context.Context, tid string, offset, limit int) (*repositories.PaginatedResult[models.Tenant], error) {
	var r []models.Tenant
	for _, t := range m.tenants {
		if t.ParentID != nil && *t.ParentID == tid {
			r = append(r, *t)
		}
	}
	return &repositories.PaginatedResult[models.Tenant]{Data: r, Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: len(r)}}, nil
}
func (m *hMockTenantRepo) GetAncestors(_ context.Context, tid string) ([]models.Tenant, error) {
	var anc []models.Tenant
	c := m.tenants[tid]
	for c != nil && c.ParentID != nil {
		p := m.tenants[*c.ParentID]
		if p == nil {
			break
		}
		anc = append(anc, *p)
		c = p
	}
	return anc, nil
}
func (m *hMockTenantRepo) ListByTierID(_ context.Context, tierID string) ([]models.Tenant, error) {
	var r []models.Tenant
	for _, t := range m.tenants {
		if t.TierID != nil && *t.TierID == tierID {
			r = append(r, *t)
		}
	}
	return r, nil
}
func (m *hMockTenantRepo) Update(_ context.Context, t *models.Tenant) error {
	m.tenants[t.ID] = t; return nil
}
func (m *hMockTenantRepo) Delete(_ context.Context, tid string, mode models.DeleteMode) error {
	if mode == models.DeleteModeCascade {
		q := []string{tid}
		for len(q) > 0 {
			id := q[0]; q = q[1:]
			for _, t := range m.tenants {
				if t.ParentID != nil && *t.ParentID == id {
					q = append(q, t.ID)
				}
			}
			delete(m.tenants, id)
		}
	} else {
		for _, t := range m.tenants {
			if t.ParentID != nil && *t.ParentID == tid {
				t.ParentID = nil
			}
		}
		delete(m.tenants, tid)
	}
	return nil
}
func (m *hMockTenantRepo) RecordTierChange(_ context.Context, c *models.TenantTierChange) error {
	m.history[c.TenantID] = append(m.history[c.TenantID], *c); return nil
}
func (m *hMockTenantRepo) GetTierHistory(_ context.Context, tid string, offset, limit int) (*repositories.PaginatedResult[models.TenantTierChange], error) {
	h := m.history[tid]
	return &repositories.PaginatedResult[models.TenantTierChange]{Data: h, Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: len(h)}}, nil
}

// --- Usage ---

type hMockUsageRepo struct {
	records map[string]*models.UsageRecord
}

func hUsageKey(tid, lk string) string { return tid + ":" + lk }

func (m *hMockUsageRepo) GetUsage(_ context.Context, tid, lk string) (*models.UsageRecord, error) {
	return m.records[hUsageKey(tid, lk)], nil
}
func (m *hMockUsageRepo) GetAllUsage(_ context.Context, tid string) ([]models.UsageRecord, error) {
	var r []models.UsageRecord
	for _, u := range m.records {
		if u.TenantID == tid {
			r = append(r, *u)
		}
	}
	return r, nil
}
func (m *hMockUsageRepo) UpsertUsage(_ context.Context, rec *models.UsageRecord) error {
	m.records[hUsageKey(rec.TenantID, rec.LimitKey)] = rec; return nil
}
func (m *hMockUsageRepo) ResetUsage(_ context.Context, tid, lk string) error {
	delete(m.records, hUsageKey(tid, lk)); return nil
}
func (m *hMockUsageRepo) ResetAllUsage(_ context.Context, tid string) error {
	for k, r := range m.records {
		if r.TenantID == tid {
			delete(m.records, k)
		}
	}
	return nil
}

// --- Transaction ---

type hMockTransactionRepo struct {
	txns map[string]*models.Transaction
}

func (m *hMockTransactionRepo) Create(_ context.Context, tx *models.Transaction) error {
	m.txns[tx.ID] = tx; return nil
}
func (m *hMockTransactionRepo) GetByID(_ context.Context, id string) (*models.Transaction, error) {
	return m.txns[id], nil
}
func (m *hMockTransactionRepo) AddOperation(_ context.Context, txID string, op models.TransactionOp) error {
	if tx, ok := m.txns[txID]; ok {
		tx.Operations = append(tx.Operations, op)
	}
	return nil
}
func (m *hMockTransactionRepo) AddLimitKeys(_ context.Context, txID string, keys []string) error {
	if tx, ok := m.txns[txID]; ok {
		tx.LimitKeys = append(tx.LimitKeys, keys...)
	}
	return nil
}
func (m *hMockTransactionRepo) UpdateStatus(_ context.Context, txID string, status models.TransactionStatus) error {
	if tx, ok := m.txns[txID]; ok {
		tx.Status = status
	}
	return nil
}
func (m *hMockTransactionRepo) GetPendingByTenantAndLimitKey(_ context.Context, tenantID, limitKey string) (*models.Transaction, error) {
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
func (m *hMockTransactionRepo) CleanExpired(_ context.Context) (int, error) { return 0, nil }
