package services

import (
	"context"
	"testing"

	"tierify/internal/config"
	"tierify/internal/errors"
	"tierify/internal/limiter"
	"tierify/internal/logger"
	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUsageService(cfg *config.Config) (UsageService, TierService, TenantService) {
	repos, _, _, _, _ := newMockRepos()
	if cfg == nil {
		cfg = &config.Config{}
	}
	log := logger.Nop()
	engine := limiter.NewEngine()
	tierSvc := NewTierService(repos, cfg, log)
	tenantSvc := NewTenantService(repos, cfg, log)
	usageSvc := NewUsageService(repos, cfg, log, engine)
	return usageSvc, tierSvc, tenantSvc
}

func setupTierAndTenant(t *testing.T, tierSvc TierService, tenantSvc TenantService) (*models.Tier, *models.Tenant) {
	t.Helper()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal, Config: models.AbsoluteConfig{MaxValue: 100}},
		{Key: "storage", Kind: models.LimitKindCumulative, Scope: models.LimitScopeLocal, Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"}},
	}
	tier, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "pro"
	tenant, err := tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	return tier, tenant
}

func TestUsageService_Check_Success(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	results, err := usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	assert.Equal(t, int64(100), results[0].Remaining)
}

func TestUsageService_Check_TierLess(t *testing.T) {
	usageSvc, _, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()

	tenant, err := tenantSvc.Create(ctx, "user-free", nil, nil, nil, nil)
	require.NoError(t, err)

	results, err := usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "anything", Amount: 999},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
}

func TestUsageService_Check_Blocked(t *testing.T) {
	usageSvc, _, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()

	tenant, err := tenantSvc.Create(ctx, "user-blocked", nil, nil, nil, nil)
	require.NoError(t, err)
	err = tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusBlocked)
	require.NoError(t, err)

	_, err = usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "tenant_blocked", appErr.Code)
	assert.True(t, appErr.Blocked)
}

func TestUsageService_Check_Suspended(t *testing.T) {
	usageSvc, _, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()

	tenant, err := tenantSvc.Create(ctx, "user-suspended", nil, nil, nil, nil)
	require.NoError(t, err)
	err = tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusSuspended)
	require.NoError(t, err)

	results, err := usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
}

func TestUsageService_Increment_Blocked(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	err := tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusBlocked)
	require.NoError(t, err)

	_, err = usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "tenant_blocked", appErr.Code)
	assert.True(t, appErr.Blocked)
}

func TestUsageService_Increment_Suspended(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	err := tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusSuspended)
	require.NoError(t, err)

	results, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	assert.Equal(t, "suspended", results[0].LimitKind)

	// Verify usage was NOT actually tracked.
	usage, err := usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Nil(t, usage)
}

func TestUsageService_Decrement_Blocked(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	err := tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusBlocked)
	require.NoError(t, err)

	_, err = usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "tenant_blocked", appErr.Code)
	assert.True(t, appErr.Blocked)
}

func TestUsageService_Decrement_Suspended(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	err := tenantSvc.UpdateStatus(ctx, tenant.ID, models.TenantStatusSuspended)
	require.NoError(t, err)

	results, err := usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	assert.Equal(t, "suspended", results[0].LimitKind)

	// Verify usage was NOT actually tracked.
	usage, err := usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Nil(t, usage)
}

func TestUsageService_Increment_Success(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	results, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	assert.Equal(t, int64(10), results[0].CurrentValue)
}

func TestUsageService_Increment_ExceedsLimit(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	// First increment to 90.
	_, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 90},
	}, nil)
	require.NoError(t, err)

	// Try to increment by 20 — exceeds 100.
	_, err = usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)
	require.Len(t, appErr.Details, 1)
	assert.True(t, appErr.Details[0].Exceeded)
}

func TestUsageService_Increment_MultiOp_AllOrNothing(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	// Increment storage to near limit.
	_, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "storage", Amount: 990},
	}, nil)
	require.NoError(t, err)

	// Multi-op: apiCalls OK, storage exceeds.
	_, err = usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
		{LimitKey: "storage", Amount: 50},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)
	assert.Len(t, appErr.Details, 2) // Both operations reported.

	// Verify apiCalls was NOT incremented (all-or-nothing).
	usage, err := usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	if usage != nil {
		assert.Equal(t, int64(0), usage.CurrentValue)
	}
}

func TestUsageService_Decrement_Success(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	_, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, nil)
	require.NoError(t, err)

	results, err := usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, int64(40), results[0].CurrentValue)
}

func TestUsageService_Decrement_BelowZero_Disallowed(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(&config.Config{AllowUsageBelowZero: false})
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	_, err := usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "usage_below_zero", appErr.Code)
}

func TestUsageService_Decrement_BelowZero_Allowed(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(&config.Config{AllowUsageBelowZero: true})
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	results, err := usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, int64(-10), results[0].CurrentValue)
}

func TestUsageService_Reset_Specific(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	_, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
		{LimitKey: "storage", Amount: 500},
	}, nil)
	require.NoError(t, err)

	err = usageSvc.Reset(ctx, tenant.ID, []string{"apiCalls"})
	require.NoError(t, err)

	// apiCalls should be reset.
	usage, err := usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Nil(t, usage)

	// storage should remain.
	storageUsage, err := usageSvc.GetUsageByKey(ctx, tenant.ID, "storage")
	require.NoError(t, err)
	assert.NotNil(t, storageUsage)
	assert.Equal(t, int64(500), storageUsage.CurrentValue)
}

func TestUsageService_Reset_All(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	_, err := usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
		{LimitKey: "storage", Amount: 500},
	}, nil)
	require.NoError(t, err)

	err = usageSvc.Reset(ctx, tenant.ID, nil)
	require.NoError(t, err)

	all, err := usageSvc.GetUsage(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestUsageService_UnknownLimitKey(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	_, tenant := setupTierAndTenant(t, tierSvc, tenantSvc)
	ctx := context.Background()

	// Checking a limit key not defined in the tier should succeed (allowed by default).
	results, err := usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "unknownKey", Amount: 1},
	}, nil)
	require.NoError(t, err)
	assert.True(t, results[0].Allowed)
}

// --- Hierarchy Tests ---

// setupHierarchy creates: org (global apiCalls:1000) -> dept -> user (local apiCalls:100)
func setupHierarchy(t *testing.T, tierSvc TierService, tenantSvc TenantService) (org, dept, user *models.Tenant) {
	t.Helper()
	ctx := context.Background()

	// Org tier: global limit shared across all descendants.
	orgLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeGlobal, Config: models.AbsoluteConfig{MaxValue: 1000}},
	}
	_, err := tierSvc.Create(ctx, "org-tier", "Organization Tier", orgLimits)
	require.NoError(t, err)

	// User tier: local limit per user.
	userLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err = tierSvc.Create(ctx, "user-tier", "User Tier", userLimits)
	require.NoError(t, err)

	// Create org tenant.
	orgKey := "org-tier"
	org, err = tenantSvc.Create(ctx, "org-1", nil, &orgKey, nil, nil)
	require.NoError(t, err)

	// Create dept tenant (no tier — tier-less).
	dept, err = tenantSvc.Create(ctx, "dept-1", &org.ID, nil, nil, nil)
	require.NoError(t, err)

	// Create user tenant under dept.
	userKey := "user-tier"
	user, err = tenantSvc.Create(ctx, "user-1", &dept.ID, &userKey, nil, nil)
	require.NoError(t, err)

	return org, dept, user
}

func TestUsageService_Hierarchy_GlobalLimitApplies(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()
	org, _, user := setupHierarchy(t, tierSvc, tenantSvc)

	// Increment user's apiCalls by 50.
	results, err := usageSvc.Increment(ctx, user.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	assert.Equal(t, int64(50), results[0].CurrentValue)

	// Verify org's usage was also incremented (global limit).
	orgUsage, err := usageSvc.GetUsageByKey(ctx, org.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, orgUsage)
	assert.Equal(t, int64(50), orgUsage.CurrentValue)
}

func TestUsageService_Hierarchy_GlobalLimitExceeded(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()
	org, _, user := setupHierarchy(t, tierSvc, tenantSvc)

	// Use up the org's global limit directly on the org.
	_, err := usageSvc.Increment(ctx, org.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 990},
	}, nil)
	require.NoError(t, err)

	// User tries to increment by 20 — user's own limit (100) is fine,
	// but org's global limit (1000) only has 10 remaining.
	_, err = usageSvc.Increment(ctx, user.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)

	// Verify org usage was not incremented (all-or-nothing).
	orgUsage, err := usageSvc.GetUsageByKey(ctx, org.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(990), orgUsage.CurrentValue)
}

func TestUsageService_Hierarchy_Check_ReflectsAncestorLimits(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()
	org, _, user := setupHierarchy(t, tierSvc, tenantSvc)

	// Use up 950 of org's 1000 global limit.
	_, err := usageSvc.Increment(ctx, org.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 950},
	}, nil)
	require.NoError(t, err)

	// Check from user's perspective — user has 100 remaining locally,
	// but org only has 50 remaining globally.
	results, err := usageSvc.Check(ctx, user.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)
	// The remaining should reflect the most restrictive (org's 50).
	assert.Equal(t, int64(50), results[0].Remaining)
}

func TestUsageService_Hierarchy_TierLessChild_AncestorGlobalApplies(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()
	org, dept, _ := setupHierarchy(t, tierSvc, tenantSvc)

	// dept is tier-less but has org as ancestor with global apiCalls limit.
	results, err := usageSvc.Check(ctx, dept.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)

	// Increment on tier-less dept should still affect org's global usage.
	results, err = usageSvc.Increment(ctx, dept.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 100},
	}, nil)
	require.NoError(t, err)
	assert.True(t, results[0].Allowed)

	// Verify org usage was incremented.
	orgUsage, err := usageSvc.GetUsageByKey(ctx, org.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, orgUsage)
	assert.Equal(t, int64(100), orgUsage.CurrentValue)
}

func TestUsageService_Hierarchy_TierLessChild_GlobalExceeded(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()
	_, dept, _ := setupHierarchy(t, tierSvc, tenantSvc)

	// Tier-less dept tries to exceed org's global limit.
	_, err := usageSvc.Increment(ctx, dept.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1001},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)
}

func TestUsageService_Hierarchy_LocalLimitNotPropagated(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()

	// Create a tier with ONLY a local limit.
	localLimits := []models.LimitDefinition{
		{Key: "localLimit", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal, Config: models.AbsoluteConfig{MaxValue: 50}},
	}
	_, err := tierSvc.Create(ctx, "parent-tier", "Parent", localLimits)
	require.NoError(t, err)

	parentKey := "parent-tier"
	parent, err := tenantSvc.Create(ctx, "parent", nil, &parentKey, nil, nil)
	require.NoError(t, err)

	// Child is tier-less.
	child, err := tenantSvc.Create(ctx, "child", &parent.ID, nil, nil, nil)
	require.NoError(t, err)

	// Child increments localLimit — since parent's limit is LOCAL, it should NOT
	// propagate to the child. Child has no own limits, so it should succeed.
	results, err := usageSvc.Increment(ctx, child.ID, []UsageOperation{
		{LimitKey: "localLimit", Amount: 999},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Allowed)

	// Verify parent's usage was NOT affected (local limits don't propagate).
	parentUsage, err := usageSvc.GetUsageByKey(ctx, parent.ID, "localLimit")
	require.NoError(t, err)
	assert.Nil(t, parentUsage)
}

func TestUsageService_Hierarchy_DeepChain(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(nil)
	ctx := context.Background()

	// Create a deep hierarchy: root -> mid -> leaf
	// root has global limit of 500
	// mid has global limit of 200
	// leaf has local limit of 100
	rootLimits := []models.LimitDefinition{
		{Key: "calls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeGlobal, Config: models.AbsoluteConfig{MaxValue: 500}},
	}
	_, err := tierSvc.Create(ctx, "root-tier", "Root", rootLimits)
	require.NoError(t, err)

	midLimits := []models.LimitDefinition{
		{Key: "calls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeGlobal, Config: models.AbsoluteConfig{MaxValue: 200}},
	}
	_, err = tierSvc.Create(ctx, "mid-tier", "Mid", midLimits)
	require.NoError(t, err)

	leafLimits := []models.LimitDefinition{
		{Key: "calls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err = tierSvc.Create(ctx, "leaf-tier", "Leaf", leafLimits)
	require.NoError(t, err)

	rootKey := "root-tier"
	root, err := tenantSvc.Create(ctx, "root", nil, &rootKey, nil, nil)
	require.NoError(t, err)

	midKey := "mid-tier"
	mid, err := tenantSvc.Create(ctx, "mid", &root.ID, &midKey, nil, nil)
	require.NoError(t, err)

	leafKey := "leaf-tier"
	leaf, err := tenantSvc.Create(ctx, "leaf", &mid.ID, &leafKey, nil, nil)
	require.NoError(t, err)

	// Leaf increments by 50 — allowed by all three limits.
	results, err := usageSvc.Increment(ctx, leaf.ID, []UsageOperation{
		{LimitKey: "calls", Amount: 50},
	}, nil)
	require.NoError(t, err)
	assert.True(t, results[0].Allowed)

	// Verify all three usage records.
	leafUsage, _ := usageSvc.GetUsageByKey(ctx, leaf.ID, "calls")
	assert.Equal(t, int64(50), leafUsage.CurrentValue)

	midUsage, _ := usageSvc.GetUsageByKey(ctx, mid.ID, "calls")
	assert.Equal(t, int64(50), midUsage.CurrentValue)

	rootUsage, _ := usageSvc.GetUsageByKey(ctx, root.ID, "calls")
	assert.Equal(t, int64(50), rootUsage.CurrentValue)

	// Another increment of 160 — leaf (100) allows it, but mid (200) would be at 210. Fail.
	_, err = usageSvc.Increment(ctx, leaf.ID, []UsageOperation{
		{LimitKey: "calls", Amount: 160},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)
}

func TestUsageService_Hierarchy_Decrement_PropagatesToAncestors(t *testing.T) {
	usageSvc, tierSvc, tenantSvc := newTestUsageService(&config.Config{AllowUsageBelowZero: false})
	ctx := context.Background()
	org, _, user := setupHierarchy(t, tierSvc, tenantSvc)

	// Increment user by 50.
	_, err := usageSvc.Increment(ctx, user.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, nil)
	require.NoError(t, err)

	// Decrement user by 20.
	results, err := usageSvc.Decrement(ctx, user.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(30), results[0].CurrentValue)

	// Org usage should also be decremented.
	orgUsage, err := usageSvc.GetUsageByKey(ctx, org.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(30), orgUsage.CurrentValue)
}
