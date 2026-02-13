package services

import (
	"context"
	"time"

	"tierify/internal/config"
	"tierify/internal/errors"
	"tierify/internal/logger"
	"tierify/internal/models"
	"tierify/internal/repositories"
)

type TenantService interface {
	Create(ctx context.Context, externalID string, parentID *string, tierKey *string, tierVersion *int, metadata map[string]any) (*models.Tenant, error)
	GetByID(ctx context.Context, id string) (*models.Tenant, error)
	GetChildren(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.Tenant], error)
	GetAncestors(ctx context.Context, tenantID string) ([]models.Tenant, error)
	AssignTier(ctx context.Context, tenantID string, tierKey string, tierVersion *int, resetUsage bool) error
	UpdateStatus(ctx context.Context, tenantID string, status models.TenantStatus) error
	Delete(ctx context.Context, tenantID string, mode models.DeleteMode) error
	GetTierHistory(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.TenantTierChange], error)
}

type tenantService struct {
	repos *repositories.Repositories
	cfg   *config.Config
	log   logger.Logger
}

func NewTenantService(repos *repositories.Repositories, cfg *config.Config, log logger.Logger) TenantService {
	return &tenantService{repos: repos, cfg: cfg, log: log}
}

func (s *tenantService) Create(ctx context.Context, externalID string, parentID *string, tierKey *string, tierVersion *int, metadata map[string]any) (*models.Tenant, error) {
	// Validate parent exists if provided.
	if parentID != nil {
		parent, err := s.repos.Tenants.GetByID(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, errors.NotFound("parent tenant", *parentID)
		}
	}

	var tierID *string
	var resolvedVersion *int

	if tierKey != nil {
		var tier *models.Tier
		var err error
		if tierVersion != nil {
			tier, err = s.repos.Tiers.GetByKeyAndVersion(ctx, *tierKey, *tierVersion)
		} else {
			tier, err = s.repos.Tiers.GetByKey(ctx, *tierKey)
		}
		if err != nil {
			return nil, err
		}
		if tier == nil {
			return nil, errors.NotFound("tier", *tierKey)
		}
		if tier.Deprecated {
			return nil, errors.DeprecatedTier(*tierKey, tier.Version)
		}
		tierID = &tier.ID
		resolvedVersion = &tier.Version
	}

	now := time.Now().UTC()
	tenant := &models.Tenant{
		ID:          models.NewID(),
		ExternalID:  externalID,
		ParentID:    parentID,
		TierID:      tierID,
		TierVersion: resolvedVersion,
		Status:      models.TenantStatusActive,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repos.Tenants.Create(ctx, tenant); err != nil {
		return nil, err
	}

	// Record initial tier assignment if tier was set.
	if tierID != nil {
		change := &models.TenantTierChange{
			ID:         models.NewID(),
			TenantID:   tenant.ID,
			NewTierID:  tierID,
			NewVersion: resolvedVersion,
			ChangedAt:  now,
		}
		if err := s.repos.Tenants.RecordTierChange(ctx, change); err != nil {
			s.log.Error("failed to record initial tier change", "tenant_id", tenant.ID, "error", err)
		}
	}

	s.log.Info("tenant created", "id", tenant.ID, "external_id", externalID)
	return tenant, nil
}

func (s *tenantService) GetByID(ctx context.Context, id string) (*models.Tenant, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", id)
	}
	return tenant, nil
}

func (s *tenantService) GetChildren(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.Tenant], error) {
	return s.repos.Tenants.GetChildren(ctx, tenantID, offset, limit)
}

func (s *tenantService) GetAncestors(ctx context.Context, tenantID string) ([]models.Tenant, error) {
	return s.repos.Tenants.GetAncestors(ctx, tenantID)
}

func (s *tenantService) AssignTier(ctx context.Context, tenantID string, tierKey string, tierVersion *int, resetUsage bool) error {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.NotFound("tenant", tenantID)
	}

	var tier *models.Tier
	if tierVersion != nil {
		tier, err = s.repos.Tiers.GetByKeyAndVersion(ctx, tierKey, *tierVersion)
	} else {
		tier, err = s.repos.Tiers.GetByKey(ctx, tierKey)
	}
	if err != nil {
		return err
	}
	if tier == nil {
		return errors.NotFound("tier", tierKey)
	}
	if tier.Deprecated {
		return errors.DeprecatedTier(tierKey, tier.Version)
	}

	previousTierID := tenant.TierID
	previousVersion := tenant.TierVersion

	tenant.TierID = &tier.ID
	tenant.TierVersion = &tier.Version
	tenant.UpdatedAt = time.Now().UTC()

	if err := s.repos.Tenants.Update(ctx, tenant); err != nil {
		return err
	}

	if resetUsage {
		if err := s.repos.Usage.ResetAllUsage(ctx, tenantID); err != nil {
			s.log.Error("failed to reset usage on tier change", "tenant_id", tenantID, "error", err)
		}
	}

	change := &models.TenantTierChange{
		ID:              models.NewID(),
		TenantID:        tenantID,
		PreviousTierID:  previousTierID,
		PreviousVersion: previousVersion,
		NewTierID:       &tier.ID,
		NewVersion:      &tier.Version,
		UsageReset:      resetUsage,
		ChangedAt:       time.Now().UTC(),
	}
	if err := s.repos.Tenants.RecordTierChange(ctx, change); err != nil {
		s.log.Error("failed to record tier change", "tenant_id", tenantID, "error", err)
	}

	s.log.Info("tenant tier assigned", "tenant_id", tenantID, "tier_key", tierKey, "version", tier.Version)
	return nil
}

func (s *tenantService) UpdateStatus(ctx context.Context, tenantID string, status models.TenantStatus) error {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.NotFound("tenant", tenantID)
	}

	tenant.Status = status
	tenant.UpdatedAt = time.Now().UTC()

	return s.repos.Tenants.Update(ctx, tenant)
}

func (s *tenantService) Delete(ctx context.Context, tenantID string, mode models.DeleteMode) error {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.NotFound("tenant", tenantID)
	}

	return s.repos.Tenants.Delete(ctx, tenantID, mode)
}

func (s *tenantService) GetTierHistory(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.TenantTierChange], error) {
	return s.repos.Tenants.GetTierHistory(ctx, tenantID, offset, limit)
}
