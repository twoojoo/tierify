package services

import (
	"context"
	"testing"

	"tierify/internal/config"
	"tierify/internal/errors"
	"tierify/internal/logger"
	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTierService() (TierService, *mockTierRepo) {
	repos, tierRepo, _, _, _ := newMockRepos()
	cfg := &config.Config{}
	svc := NewTierService(repos, cfg, logger.Nop())
	return svc, tierRepo
}

func TestTierService_Create(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}

	tier, err := svc.Create(ctx, "basic", "Basic Plan", limits)
	require.NoError(t, err)
	assert.Equal(t, "basic", tier.Key)
	assert.Equal(t, "Basic Plan", tier.Name)
	assert.Equal(t, 1, tier.Version)
	assert.False(t, tier.Deprecated)
	assert.NotEmpty(t, tier.ID)
}

func TestTierService_Create_InvalidLimit(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: -1}},
	}

	_, err := svc.Create(ctx, "bad", "Bad Plan", limits)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "bad_request", appErr.Code)
}

func TestTierService_GetByKey(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tier, err := svc.GetByKey(ctx, "pro")
	require.NoError(t, err)
	assert.Equal(t, "pro", tier.Key)
}

func TestTierService_GetByKey_NotFound(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	_, err := svc.GetByKey(ctx, "nonexistent")
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "not_found", appErr.Code)
}

func TestTierService_Update(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	newLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 200}},
	}
	updated, err := svc.Update(ctx, "pro", newLimits, models.TierUpdateOptions{DeprecatePrevious: "none"})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
}

func TestTierService_Update_DeprecateLatest(t *testing.T) {
	svc, tierRepo := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	newLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 200}},
	}
	_, err = svc.Update(ctx, "pro", newLimits, models.TierUpdateOptions{DeprecatePrevious: "latest"})
	require.NoError(t, err)

	v1 := tierRepo.tiers[tierKey("pro", 1)]
	assert.True(t, v1.Deprecated)
}

func TestTierService_Update_DeprecateAll(t *testing.T) {
	svc, tierRepo := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	_, err = svc.Update(ctx, "pro", limits, models.TierUpdateOptions{DeprecatePrevious: "none"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, "pro", limits, models.TierUpdateOptions{DeprecatePrevious: "all"})
	require.NoError(t, err)

	assert.True(t, tierRepo.tiers[tierKey("pro", 1)].Deprecated)
	assert.True(t, tierRepo.tiers[tierKey("pro", 2)].Deprecated)
	assert.False(t, tierRepo.tiers[tierKey("pro", 3)].Deprecated)
}

func TestTierService_Deprecate(t *testing.T) {
	svc, tierRepo := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	err = svc.Deprecate(ctx, "pro", 1)
	require.NoError(t, err)
	assert.True(t, tierRepo.tiers[tierKey("pro", 1)].Deprecated)
}

func TestTierService_GetLimits(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
		{Key: "storage", Kind: models.LimitKindCumulative, Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	result, err := svc.GetLimits(ctx, "pro", 1)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestTierService_GetLimit(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	result, err := svc.GetLimit(ctx, "pro", 1, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, "apiCalls", result.Key)
}

func TestTierService_GetLimit_NotFound(t *testing.T) {
	svc, _ := newTestTierService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := svc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	_, err = svc.GetLimit(ctx, "pro", 1, "nonexistent")
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "not_found", appErr.Code)
}

// --- Tier Migration Tests ---

func newTestTierServiceFull() (TierService, TenantService, *mockTenantRepo, *mockUsageRepo) {
	repos, _, tenantRepo, usageRepo, _ := newMockRepos()
	cfg := &config.Config{}
	log := logger.Nop()
	tierSvc := NewTierService(repos, cfg, log)
	tenantSvc := NewTenantService(repos, cfg, log)
	return tierSvc, tenantSvc, tenantRepo, usageRepo
}

func TestTierService_Update_MigrateExisting(t *testing.T) {
	tierSvc, tenantSvc, tenantRepo, _ := newTestTierServiceFull()
	ctx := context.Background()

	// Create v1 tier.
	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	v1, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	// Create tenants on v1.
	tierKey := "pro"
	t1, err := tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)
	t2, err := tenantSvc.Create(ctx, "user-2", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	// Both tenants should be on v1.
	assert.Equal(t, &v1.ID, t1.TierID)
	assert.Equal(t, 1, *t1.TierVersion)

	// Update tier with migration enabled.
	newLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 200}},
	}
	v2, err := tierSvc.Update(ctx, "pro", newLimits, models.TierUpdateOptions{
		MigrateExisting:   true,
		DeprecatePrevious: "latest",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, v2.Version)

	// Both tenants should now be on v2.
	migrated1 := tenantRepo.tenants[t1.ID]
	require.NotNil(t, migrated1)
	assert.Equal(t, &v2.ID, migrated1.TierID)
	assert.Equal(t, 2, *migrated1.TierVersion)

	migrated2 := tenantRepo.tenants[t2.ID]
	require.NotNil(t, migrated2)
	assert.Equal(t, &v2.ID, migrated2.TierID)
	assert.Equal(t, 2, *migrated2.TierVersion)
}

func TestTierService_Update_MigrateExisting_WithUsageReset(t *testing.T) {
	tierSvc, tenantSvc, _, usageRepo := newTestTierServiceFull()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "pro"
	tenant, err := tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	// Add some usage.
	usageRepo.records[usageKey(tenant.ID, "apiCalls")] = &models.UsageRecord{
		ID:           models.NewID(),
		TenantID:     tenant.ID,
		LimitKey:     "apiCalls",
		CurrentValue: 50,
	}

	// Update with migration + reset.
	newLimits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 200}},
	}
	_, err = tierSvc.Update(ctx, "pro", newLimits, models.TierUpdateOptions{
		MigrateExisting:   true,
		ResetUsage:        true,
		DeprecatePrevious: "all",
	})
	require.NoError(t, err)

	// Usage should be cleared.
	assert.Nil(t, usageRepo.records[usageKey(tenant.ID, "apiCalls")])
}

func TestTierService_Update_MigrateExisting_RecordsHistory(t *testing.T) {
	tierSvc, tenantSvc, _, _ := newTestTierServiceFull()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "pro"
	tenant, err := tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	// Update with migration.
	_, err = tierSvc.Update(ctx, "pro", limits, models.TierUpdateOptions{
		MigrateExisting:   true,
		DeprecatePrevious: "none",
	})
	require.NoError(t, err)

	// Tenant should have 2 history entries: initial assignment + migration.
	history, err := tenantSvc.GetTierHistory(ctx, tenant.ID, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, history.Pagination.Total)
}

func TestTierService_Update_NoMigrate(t *testing.T) {
	tierSvc, tenantSvc, tenantRepo, _ := newTestTierServiceFull()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	v1, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "pro"
	tenant, err := tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	// Update WITHOUT migration.
	_, err = tierSvc.Update(ctx, "pro", limits, models.TierUpdateOptions{
		MigrateExisting:   false,
		DeprecatePrevious: "none",
	})
	require.NoError(t, err)

	// Tenant should still be on v1.
	current := tenantRepo.tenants[tenant.ID]
	assert.Equal(t, &v1.ID, current.TierID)
	assert.Equal(t, 1, *current.TierVersion)
}
