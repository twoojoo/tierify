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

func newTestTenantService() (TenantService, TierService, *mockTenantRepo) {
	repos, _, tenantRepo, _, _ := newMockRepos()
	cfg := &config.Config{}
	log := logger.Nop()
	tenantSvc := NewTenantService(repos, cfg, log)
	tierSvc := NewTierService(repos, cfg, log)
	return tenantSvc, tierSvc, tenantRepo
}

func TestTenantService_Create_TierLess(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	tenant, err := svc.Create(ctx, "user-1", nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "user-1", tenant.ExternalID)
	assert.Nil(t, tenant.TierID)
	assert.Equal(t, models.TenantStatusActive, tenant.Status)
}

func TestTenantService_Create_WithTier(t *testing.T) {
	svc, tierSvc, _ := newTestTenantService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 100}},
	}
	_, err := tierSvc.Create(ctx, "basic", "Basic", limits)
	require.NoError(t, err)

	tierKey := "basic"
	tenant, err := svc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, tenant.TierID)
	assert.NotNil(t, tenant.TierVersion)
	assert.Equal(t, 1, *tenant.TierVersion)
}

func TestTenantService_Create_DeprecatedTier(t *testing.T) {
	svc, tierSvc, _ := newTestTenantService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := tierSvc.Create(ctx, "old", "Old", limits)
	require.NoError(t, err)
	err = tierSvc.Deprecate(ctx, "old", 1)
	require.NoError(t, err)

	tierKey := "old"
	version := 1
	_, err = svc.Create(ctx, "user-1", nil, &tierKey, &version, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "deprecated_tier", appErr.Code)
}

func TestTenantService_Create_WithParent(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	parent, err := svc.Create(ctx, "org-1", nil, nil, nil, nil)
	require.NoError(t, err)

	child, err := svc.Create(ctx, "user-1", &parent.ID, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, parent.ID, *child.ParentID)
}

func TestTenantService_Create_InvalidParent(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	badID := "nonexistent"
	_, err := svc.Create(ctx, "user-1", &badID, nil, nil, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "not_found", appErr.Code)
}

func TestTenantService_GetByID(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "user-1", nil, nil, nil, nil)
	require.NoError(t, err)

	fetched, err := svc.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
}

func TestTenantService_GetByID_NotFound(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent")
	require.Error(t, err)
}

func TestTenantService_AssignTier(t *testing.T) {
	svc, tierSvc, _ := newTestTenantService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tenant, err := svc.Create(ctx, "user-1", nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, tenant.TierID)

	err = svc.AssignTier(ctx, tenant.ID, "pro", nil, false)
	require.NoError(t, err)

	fetched, err := svc.GetByID(ctx, tenant.ID)
	require.NoError(t, err)
	assert.NotNil(t, fetched.TierID)
}

func TestTenantService_UpdateStatus(t *testing.T) {
	svc, _, _ := newTestTenantService()
	ctx := context.Background()

	tenant, err := svc.Create(ctx, "user-1", nil, nil, nil, nil)
	require.NoError(t, err)

	err = svc.UpdateStatus(ctx, tenant.ID, models.TenantStatusBlocked)
	require.NoError(t, err)

	fetched, err := svc.GetByID(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TenantStatusBlocked, fetched.Status)
}

func TestTenantService_Delete_Cascade(t *testing.T) {
	svc, _, tenantRepo := newTestTenantService()
	ctx := context.Background()

	parent, err := svc.Create(ctx, "org", nil, nil, nil, nil)
	require.NoError(t, err)
	_, err = svc.Create(ctx, "child1", &parent.ID, nil, nil, nil)
	require.NoError(t, err)
	_, err = svc.Create(ctx, "child2", &parent.ID, nil, nil, nil)
	require.NoError(t, err)

	err = svc.Delete(ctx, parent.ID, models.DeleteModeCascade)
	require.NoError(t, err)
	assert.Empty(t, tenantRepo.tenants)
}

func TestTenantService_Delete_Orphan(t *testing.T) {
	svc, _, tenantRepo := newTestTenantService()
	ctx := context.Background()

	parent, err := svc.Create(ctx, "org", nil, nil, nil, nil)
	require.NoError(t, err)
	child, err := svc.Create(ctx, "child1", &parent.ID, nil, nil, nil)
	require.NoError(t, err)

	err = svc.Delete(ctx, parent.ID, models.DeleteModeOrphan)
	require.NoError(t, err)

	// Parent deleted.
	assert.Nil(t, tenantRepo.tenants[parent.ID])
	// Child still exists, orphaned.
	orphan := tenantRepo.tenants[child.ID]
	assert.NotNil(t, orphan)
	assert.Nil(t, orphan.ParentID)
}

func TestTenantService_TierHistory(t *testing.T) {
	svc, tierSvc, _ := newTestTenantService()
	ctx := context.Background()

	limits := []models.LimitDefinition{
		{Key: "x", Kind: models.LimitKindAbsolute, Config: models.AbsoluteConfig{MaxValue: 1}},
	}
	_, err := tierSvc.Create(ctx, "basic", "Basic", limits)
	require.NoError(t, err)
	_, err = tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "basic"
	tenant, err := svc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	err = svc.AssignTier(ctx, tenant.ID, "pro", nil, false)
	require.NoError(t, err)

	history, err := svc.GetTierHistory(ctx, tenant.ID, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, history.Pagination.Total) // Initial assignment + change.
}
