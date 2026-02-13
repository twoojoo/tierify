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

type TierService interface {
	Create(ctx context.Context, key, name string, limits []models.LimitDefinition) (*models.Tier, error)
	GetByKey(ctx context.Context, key string) (*models.Tier, error)
	GetByKeyAndVersion(ctx context.Context, key string, version int) (*models.Tier, error)
	ListVersions(ctx context.Context, key string, offset, limit int) (*repositories.PaginatedResult[models.Tier], error)
	List(ctx context.Context, filters models.TierFilters) (*repositories.PaginatedResult[models.Tier], error)
	Update(ctx context.Context, key string, limits []models.LimitDefinition, opts models.TierUpdateOptions) (*models.Tier, error)
	Deprecate(ctx context.Context, key string, version int) error
	GetLimits(ctx context.Context, tierKey string, version int) ([]models.LimitDefinition, error)
	GetLimit(ctx context.Context, tierKey string, version int, limitKey string) (*models.LimitDefinition, error)
}

type tierService struct {
	repos  *repositories.Repositories
	cfg    *config.Config
	log    logger.Logger
}

func NewTierService(repos *repositories.Repositories, cfg *config.Config, log logger.Logger) TierService {
	return &tierService{repos: repos, cfg: cfg, log: log}
}

func (s *tierService) Create(ctx context.Context, key, name string, limits []models.LimitDefinition) (*models.Tier, error) {
	for i := range limits {
		if err := limits[i].Config.Validate(); err != nil {
			return nil, errors.BadRequest("limit " + limits[i].Key + ": " + err.Error())
		}
		limits[i].ID = models.NewID()
	}

	now := time.Now().UTC()
	tier := &models.Tier{
		ID:        models.NewID(),
		Key:       key,
		Name:      name,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for i := range limits {
		limits[i].TierID = tier.ID
		limits[i].CreatedAt = now
	}

	if err := s.repos.Tiers.Create(ctx, tier, limits); err != nil {
		return nil, err
	}

	s.log.Info("tier created", "key", key, "version", 1)
	return tier, nil
}

func (s *tierService) GetByKey(ctx context.Context, key string) (*models.Tier, error) {
	tier, err := s.repos.Tiers.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if tier == nil {
		return nil, errors.NotFound("tier", key)
	}
	return tier, nil
}

func (s *tierService) GetByKeyAndVersion(ctx context.Context, key string, version int) (*models.Tier, error) {
	tier, err := s.repos.Tiers.GetByKeyAndVersion(ctx, key, version)
	if err != nil {
		return nil, err
	}
	if tier == nil {
		return nil, errors.NotFound("tier", key)
	}
	return tier, nil
}

func (s *tierService) ListVersions(ctx context.Context, key string, offset, limit int) (*repositories.PaginatedResult[models.Tier], error) {
	return s.repos.Tiers.ListVersions(ctx, key, offset, limit)
}

func (s *tierService) List(ctx context.Context, filters models.TierFilters) (*repositories.PaginatedResult[models.Tier], error) {
	return s.repos.Tiers.List(ctx, filters)
}

func (s *tierService) Update(ctx context.Context, key string, limits []models.LimitDefinition, opts models.TierUpdateOptions) (*models.Tier, error) {
	// Get the current latest version.
	current, err := s.repos.Tiers.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.NotFound("tier", key)
	}

	for i := range limits {
		if err := limits[i].Config.Validate(); err != nil {
			return nil, errors.BadRequest("limit " + limits[i].Key + ": " + err.Error())
		}
		limits[i].ID = models.NewID()
	}

	now := time.Now().UTC()
	newTier := &models.Tier{
		ID:        models.NewID(),
		Key:       key,
		Name:      current.Name,
		Version:   current.Version + 1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for i := range limits {
		limits[i].TierID = newTier.ID
		limits[i].CreatedAt = now
	}

	if err := s.repos.Tiers.Create(ctx, newTier, limits); err != nil {
		return nil, err
	}

	// Handle deprecation of previous versions.
	switch opts.DeprecatePrevious {
	case "latest":
		if err := s.repos.Tiers.Deprecate(ctx, key, current.Version); err != nil {
			s.log.Error("failed to deprecate previous version", "key", key, "version", current.Version, "error", err)
		}
	case "all":
		for v := 1; v <= current.Version; v++ {
			if err := s.repos.Tiers.Deprecate(ctx, key, v); err != nil {
				s.log.Error("failed to deprecate version", "key", key, "version", v, "error", err)
			}
		}
	}

	// Migrate existing tenants to the new version.
	if opts.MigrateExisting {
		if err := s.migrateTenantsToVersion(ctx, key, current.Version, newTier, opts.ResetUsage); err != nil {
			s.log.Error("failed to migrate tenants", "key", key, "error", err)
		}
	}

	s.log.Info("tier updated", "key", key, "version", newTier.Version)
	return newTier, nil
}

// migrateTenantsToVersion finds all tenants on any previous version of this tier
// and updates them to the new version.
func (s *tierService) migrateTenantsToVersion(ctx context.Context, tierKey string, upToVersion int, newTier *models.Tier, resetUsage bool) error {
	now := time.Now().UTC()

	// Iterate through all previous versions and migrate their tenants.
	for v := 1; v <= upToVersion; v++ {
		oldTier, err := s.repos.Tiers.GetByKeyAndVersion(ctx, tierKey, v)
		if err != nil {
			return err
		}
		if oldTier == nil {
			continue
		}

		tenants, err := s.repos.Tenants.ListByTierID(ctx, oldTier.ID)
		if err != nil {
			return err
		}

		for i := range tenants {
			tenant := &tenants[i]
			previousTierID := tenant.TierID
			previousVersion := tenant.TierVersion

			tenant.TierID = &newTier.ID
			tenant.TierVersion = &newTier.Version
			tenant.UpdatedAt = now

			if err := s.repos.Tenants.Update(ctx, tenant); err != nil {
				s.log.Error("failed to migrate tenant", "tenant_id", tenant.ID, "error", err)
				continue
			}

			if resetUsage {
				if err := s.repos.Usage.ResetAllUsage(ctx, tenant.ID); err != nil {
					s.log.Error("failed to reset usage for migrated tenant", "tenant_id", tenant.ID, "error", err)
				}
			}

			change := &models.TenantTierChange{
				ID:              models.NewID(),
				TenantID:        tenant.ID,
				PreviousTierID:  previousTierID,
				PreviousVersion: previousVersion,
				NewTierID:       &newTier.ID,
				NewVersion:      &newTier.Version,
				UsageReset:      resetUsage,
				ChangedAt:       now,
			}
			if err := s.repos.Tenants.RecordTierChange(ctx, change); err != nil {
				s.log.Error("failed to record migration tier change", "tenant_id", tenant.ID, "error", err)
			}
		}
	}

	return nil
}

func (s *tierService) Deprecate(ctx context.Context, key string, version int) error {
	tier, err := s.repos.Tiers.GetByKeyAndVersion(ctx, key, version)
	if err != nil {
		return err
	}
	if tier == nil {
		return errors.NotFound("tier version", key)
	}
	return s.repos.Tiers.Deprecate(ctx, key, version)
}

func (s *tierService) GetLimits(ctx context.Context, tierKey string, version int) ([]models.LimitDefinition, error) {
	tier, err := s.repos.Tiers.GetByKeyAndVersion(ctx, tierKey, version)
	if err != nil {
		return nil, err
	}
	if tier == nil {
		return nil, errors.NotFound("tier", tierKey)
	}
	return s.repos.LimitDefinitions.GetByTier(ctx, tier.ID)
}

func (s *tierService) GetLimit(ctx context.Context, tierKey string, version int, limitKey string) (*models.LimitDefinition, error) {
	tier, err := s.repos.Tiers.GetByKeyAndVersion(ctx, tierKey, version)
	if err != nil {
		return nil, err
	}
	if tier == nil {
		return nil, errors.NotFound("tier", tierKey)
	}
	def, err := s.repos.LimitDefinitions.GetByTierAndKey(ctx, tier.ID, limitKey)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, errors.NotFound("limit", limitKey)
	}
	return def, nil
}
