package sqlite

import (
	"context"
	"testing"
	"time"

	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Tier Integration Tests ---

func TestSQLite_TierCRUD(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	// Create.
	now := time.Now().UTC()
	tier := &models.Tier{
		ID: models.NewID(), Key: "pro", Name: "Pro Plan", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	limits := []models.LimitDefinition{
		{ID: models.NewID(), TierID: tier.ID, Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal,
			Config: models.AbsoluteConfig{MaxValue: 100}, CreatedAt: now},
	}
	err = repos.Tiers.Create(ctx, tier, limits)
	require.NoError(t, err)

	// GetByKey.
	fetched, err := repos.Tiers.GetByKey(ctx, "pro")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "pro", fetched.Key)
	assert.Equal(t, 1, fetched.Version)

	// GetByKeyAndVersion.
	fetched, err = repos.Tiers.GetByKeyAndVersion(ctx, "pro", 1)
	require.NoError(t, err)
	require.NotNil(t, fetched)

	// GetByKey not found.
	fetched, err = repos.Tiers.GetByKey(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, fetched)

	// List.
	result, err := repos.Tiers.List(ctx, models.TierFilters{Offset: 0, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Pagination.Total)

	// Deprecate.
	err = repos.Tiers.Deprecate(ctx, "pro", 1)
	require.NoError(t, err)

	fetched, err = repos.Tiers.GetByKey(ctx, "pro")
	require.NoError(t, err)
	assert.Nil(t, fetched, "deprecated tiers should not be returned by GetByKey")

	fetched, err = repos.Tiers.GetByKeyAndVersion(ctx, "pro", 1)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.True(t, fetched.Deprecated)
}

func TestSQLite_LimitDefinitions(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	tier := &models.Tier{ID: models.NewID(), Key: "pro", Name: "Pro", Version: 1, CreatedAt: now, UpdatedAt: now}
	limits := []models.LimitDefinition{
		{ID: models.NewID(), TierID: tier.ID, Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal,
			Config: models.AbsoluteConfig{MaxValue: 100}, CreatedAt: now},
		{ID: models.NewID(), TierID: tier.ID, Key: "storage", Kind: models.LimitKindCumulative, Scope: models.LimitScopeGlobal,
			Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"}, CreatedAt: now},
	}
	err = repos.Tiers.Create(ctx, tier, limits)
	require.NoError(t, err)

	// GetByTier.
	fetched, err := repos.LimitDefinitions.GetByTier(ctx, tier.ID)
	require.NoError(t, err)
	assert.Len(t, fetched, 2)

	// GetByTierAndKey.
	limit, err := repos.LimitDefinitions.GetByTierAndKey(ctx, tier.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, limit)
	assert.Equal(t, models.LimitKindAbsolute, limit.Kind)
	assert.Equal(t, models.LimitScopeLocal, limit.Scope)

	cfg, ok := limit.Config.(models.AbsoluteConfig)
	require.True(t, ok)
	assert.Equal(t, int64(100), cfg.MaxValue)

	// Not found.
	limit, err = repos.LimitDefinitions.GetByTierAndKey(ctx, tier.ID, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, limit)
}

// --- Tenant Integration Tests ---

func TestSQLite_TenantCRUD(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	tenant := &models.Tenant{
		ID: models.NewID(), ExternalID: "user-1", Status: models.TenantStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	err = repos.Tenants.Create(ctx, tenant)
	require.NoError(t, err)

	// GetByID.
	fetched, err := repos.Tenants.GetByID(ctx, tenant.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "user-1", fetched.ExternalID)

	// GetByExternalID.
	fetched, err = repos.Tenants.GetByExternalID(ctx, "user-1")
	require.NoError(t, err)
	require.NotNil(t, fetched)

	// Update.
	fetched.Status = models.TenantStatusBlocked
	fetched.UpdatedAt = time.Now().UTC()
	err = repos.Tenants.Update(ctx, fetched)
	require.NoError(t, err)

	refetch, err := repos.Tenants.GetByID(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TenantStatusBlocked, refetch.Status)
}

func TestSQLite_TenantHierarchy(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()

	org := &models.Tenant{ID: models.NewID(), ExternalID: "org", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	dept := &models.Tenant{ID: models.NewID(), ExternalID: "dept", ParentID: &org.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	user := &models.Tenant{ID: models.NewID(), ExternalID: "user", ParentID: &dept.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}

	require.NoError(t, repos.Tenants.Create(ctx, org))
	require.NoError(t, repos.Tenants.Create(ctx, dept))
	require.NoError(t, repos.Tenants.Create(ctx, user))

	// GetChildren.
	children, err := repos.Tenants.GetChildren(ctx, org.ID, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, children.Pagination.Total)
	assert.Equal(t, "dept", children.Data[0].ExternalID)

	// GetAncestors.
	ancestors, err := repos.Tenants.GetAncestors(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, ancestors, 2) // dept + org
}

func TestSQLite_TenantDelete_Cascade(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	org := &models.Tenant{ID: models.NewID(), ExternalID: "org", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	child1 := &models.Tenant{ID: models.NewID(), ExternalID: "c1", ParentID: &org.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	child2 := &models.Tenant{ID: models.NewID(), ExternalID: "c2", ParentID: &org.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}

	require.NoError(t, repos.Tenants.Create(ctx, org))
	require.NoError(t, repos.Tenants.Create(ctx, child1))
	require.NoError(t, repos.Tenants.Create(ctx, child2))

	err = repos.Tenants.Delete(ctx, org.ID, models.DeleteModeCascade)
	require.NoError(t, err)

	fetched, _ := repos.Tenants.GetByID(ctx, org.ID)
	assert.Nil(t, fetched)
	fetched, _ = repos.Tenants.GetByID(ctx, child1.ID)
	assert.Nil(t, fetched)
}

func TestSQLite_TenantDelete_Orphan(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	org := &models.Tenant{ID: models.NewID(), ExternalID: "org", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	child := &models.Tenant{ID: models.NewID(), ExternalID: "child", ParentID: &org.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}

	require.NoError(t, repos.Tenants.Create(ctx, org))
	require.NoError(t, repos.Tenants.Create(ctx, child))

	err = repos.Tenants.Delete(ctx, org.ID, models.DeleteModeOrphan)
	require.NoError(t, err)

	fetched, _ := repos.Tenants.GetByID(ctx, org.ID)
	assert.Nil(t, fetched)

	orphan, err := repos.Tenants.GetByID(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, orphan)
	assert.Nil(t, orphan.ParentID)
}

// --- Usage Integration Tests ---

func TestSQLite_UsageCRUD(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	tenant := &models.Tenant{ID: models.NewID(), ExternalID: "user-1", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repos.Tenants.Create(ctx, tenant))

	// Upsert (create).
	record := &models.UsageRecord{
		ID: models.NewID(), TenantID: tenant.ID, LimitKey: "apiCalls",
		CurrentValue: 50, UpdatedAt: now,
	}
	err = repos.Usage.UpsertUsage(ctx, record)
	require.NoError(t, err)

	// Get.
	fetched, err := repos.Usage.GetUsage(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, int64(50), fetched.CurrentValue)

	// Upsert (update).
	record.CurrentValue = 60
	record.UpdatedAt = time.Now().UTC()
	err = repos.Usage.UpsertUsage(ctx, record)
	require.NoError(t, err)

	fetched, err = repos.Usage.GetUsage(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(60), fetched.CurrentValue)

	// GetAll.
	record2 := &models.UsageRecord{
		ID: models.NewID(), TenantID: tenant.ID, LimitKey: "storage",
		CurrentValue: 100, UpdatedAt: now,
	}
	repos.Usage.UpsertUsage(ctx, record2)

	all, err := repos.Usage.GetAllUsage(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// Reset specific.
	err = repos.Usage.ResetUsage(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)

	fetched, err = repos.Usage.GetUsage(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Nil(t, fetched)

	// Reset all.
	err = repos.Usage.ResetAllUsage(ctx, tenant.ID)
	require.NoError(t, err)

	all, err = repos.Usage.GetAllUsage(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestSQLite_UsageWithTimeWindow(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	windowEnd := now.Add(time.Hour)

	tenant := &models.Tenant{ID: models.NewID(), ExternalID: "user-1", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repos.Tenants.Create(ctx, tenant))

	record := &models.UsageRecord{
		ID: models.NewID(), TenantID: tenant.ID, LimitKey: "apiCalls",
		CurrentValue: 50, WindowStart: &now, WindowEnd: &windowEnd, UpdatedAt: now,
	}
	err = repos.Usage.UpsertUsage(ctx, record)
	require.NoError(t, err)

	fetched, err := repos.Usage.GetUsage(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.NotNil(t, fetched.WindowStart)
	require.NotNil(t, fetched.WindowEnd)
}

// --- Transaction Integration Tests ---

func TestSQLite_TransactionCRUD(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	ttl := 30 * time.Second
	expiresAt := now.Add(ttl)

	txn := &models.Transaction{
		ID: "tx-1", TenantID: "tenant-1", LimitKeys: []string{"apiCalls"},
		Status: models.TransactionStatusPending, TTL: &ttl,
		CreatedAt: now, ExpiresAt: &expiresAt,
	}
	err = repos.Transactions.Create(ctx, txn)
	require.NoError(t, err)

	// Get.
	fetched, err := repos.Transactions.GetByID(ctx, "tx-1")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "tx-1", fetched.ID)
	assert.Equal(t, models.TransactionStatusPending, fetched.Status)

	// AddOperation.
	err = repos.Transactions.AddOperation(ctx, "tx-1", models.TransactionOp{
		LimitKey: "apiCalls", Operation: "increment", Amount: 10,
	})
	require.NoError(t, err)

	fetched, _ = repos.Transactions.GetByID(ctx, "tx-1")
	assert.Len(t, fetched.Operations, 1)

	// AddLimitKeys.
	err = repos.Transactions.AddLimitKeys(ctx, "tx-1", []string{"storage"})
	require.NoError(t, err)

	fetched, _ = repos.Transactions.GetByID(ctx, "tx-1")
	assert.Len(t, fetched.LimitKeys, 2)

	// UpdateStatus.
	err = repos.Transactions.UpdateStatus(ctx, "tx-1", models.TransactionStatusCommitted)
	require.NoError(t, err)

	fetched, _ = repos.Transactions.GetByID(ctx, "tx-1")
	assert.Equal(t, models.TransactionStatusCommitted, fetched.Status)

	// GetPendingByTenantAndLimitKey — should not find committed.
	pending, err := repos.Transactions.GetPendingByTenantAndLimitKey(ctx, "tenant-1", "apiCalls")
	require.NoError(t, err)
	assert.Nil(t, pending)
}

// --- Tier History Integration Test ---

func TestSQLite_TierHistory(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	tenant := &models.Tenant{ID: models.NewID(), ExternalID: "user-1", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repos.Tenants.Create(ctx, tenant))

	change := &models.TenantTierChange{
		ID: models.NewID(), TenantID: tenant.ID,
		NewTierID: strPtr("tier-1"), NewVersion: intPtr(1),
		ChangedAt: now,
	}
	err = repos.Tenants.RecordTierChange(ctx, change)
	require.NoError(t, err)

	history, err := repos.Tenants.GetTierHistory(ctx, tenant.ID, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, history.Pagination.Total)
}

// --- ListByTierID Integration Test ---

func TestSQLite_ListByTierID(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()
	tier := &models.Tier{ID: models.NewID(), Key: "pro", Name: "Pro", Version: 1, CreatedAt: now, UpdatedAt: now}
	err = repos.Tiers.Create(ctx, tier, nil)
	require.NoError(t, err)

	t1 := &models.Tenant{ID: models.NewID(), ExternalID: "u1", TierID: &tier.ID, TierVersion: intPtr(1), Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	t2 := &models.Tenant{ID: models.NewID(), ExternalID: "u2", TierID: &tier.ID, TierVersion: intPtr(1), Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	t3 := &models.Tenant{ID: models.NewID(), ExternalID: "u3", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now} // tier-less

	require.NoError(t, repos.Tenants.Create(ctx, t1))
	require.NoError(t, repos.Tenants.Create(ctx, t2))
	require.NoError(t, repos.Tenants.Create(ctx, t3))

	tenants, err := repos.Tenants.ListByTierID(ctx, tier.ID)
	require.NoError(t, err)
	assert.Len(t, tenants, 2)
}

// --- Hierarchy with Global Limits Integration Test ---

func TestSQLite_HierarchyAncestors_Deep(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create a 4-level hierarchy: root -> l1 -> l2 -> l3
	root := &models.Tenant{ID: models.NewID(), ExternalID: "root", Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	l1 := &models.Tenant{ID: models.NewID(), ExternalID: "l1", ParentID: &root.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	l2 := &models.Tenant{ID: models.NewID(), ExternalID: "l2", ParentID: &l1.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	l3 := &models.Tenant{ID: models.NewID(), ExternalID: "l3", ParentID: &l2.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}

	require.NoError(t, repos.Tenants.Create(ctx, root))
	require.NoError(t, repos.Tenants.Create(ctx, l1))
	require.NoError(t, repos.Tenants.Create(ctx, l2))
	require.NoError(t, repos.Tenants.Create(ctx, l3))

	// l3 should have 3 ancestors: l2, l1, root
	ancestors, err := repos.Tenants.GetAncestors(ctx, l3.ID)
	require.NoError(t, err)
	assert.Len(t, ancestors, 3)

	// Root should have 0 ancestors.
	ancestors, err = repos.Tenants.GetAncestors(ctx, root.ID)
	require.NoError(t, err)
	assert.Len(t, ancestors, 0)

	// l1's children should be just l2.
	children, err := repos.Tenants.GetChildren(ctx, l1.ID, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, children.Pagination.Total)
	assert.Equal(t, "l2", children.Data[0].ExternalID)
}

func TestSQLite_HierarchyUsage_GlobalScope(t *testing.T) {
	repos, err := NewRepositories(":memory:")
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create org tier with global limit.
	orgTier := &models.Tier{ID: models.NewID(), Key: "org-tier", Name: "Org", Version: 1, CreatedAt: now, UpdatedAt: now}
	orgLimits := []models.LimitDefinition{
		{ID: models.NewID(), TierID: orgTier.ID, Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeGlobal,
			Config: models.AbsoluteConfig{MaxValue: 1000}, CreatedAt: now},
	}
	require.NoError(t, repos.Tiers.Create(ctx, orgTier, orgLimits))

	// Create tenants in hierarchy.
	org := &models.Tenant{ID: models.NewID(), ExternalID: "org", TierID: &orgTier.ID, TierVersion: intPtr(1), Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	user := &models.Tenant{ID: models.NewID(), ExternalID: "user", ParentID: &org.ID, Status: models.TenantStatusActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repos.Tenants.Create(ctx, org))
	require.NoError(t, repos.Tenants.Create(ctx, user))

	// Verify we can fetch limit definitions with correct scope.
	limits, err := repos.LimitDefinitions.GetByTier(ctx, orgTier.ID)
	require.NoError(t, err)
	require.Len(t, limits, 1)
	assert.Equal(t, models.LimitScopeGlobal, limits[0].Scope)

	// Verify ancestors chain works for the user.
	ancestors, err := repos.Tenants.GetAncestors(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, ancestors, 1)
	assert.Equal(t, org.ID, ancestors[0].ID)
	assert.NotNil(t, ancestors[0].TierID)

	// Set usage on org and verify it persists.
	orgUsage := &models.UsageRecord{
		ID: models.NewID(), TenantID: org.ID, LimitKey: "apiCalls",
		CurrentValue: 500, UpdatedAt: now,
	}
	require.NoError(t, repos.Usage.UpsertUsage(ctx, orgUsage))

	fetched, err := repos.Usage.GetUsage(ctx, org.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, int64(500), fetched.CurrentValue)
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
