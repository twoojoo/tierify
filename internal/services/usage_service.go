package services

import (
	"context"
	"time"

	"tierify/internal/config"
	"tierify/internal/errors"
	"tierify/internal/limiter"
	"tierify/internal/logger"
	"tierify/internal/models"
	"tierify/internal/repositories"
)

type UsageOperation struct {
	LimitKey string `json:"limit_key"`
	Amount   int64  `json:"amount"`
}

// TxContext carries optional transaction information for usage operations.
type TxContext struct {
	TransactionID string
	TTL           *time.Duration
}

type UsageService interface {
	Check(ctx context.Context, tenantID string, operations []UsageOperation, tx *TxContext) ([]limiter.CheckResult, error)
	Increment(ctx context.Context, tenantID string, operations []UsageOperation, tx *TxContext) ([]limiter.CheckResult, error)
	Decrement(ctx context.Context, tenantID string, operations []UsageOperation, tx *TxContext) ([]limiter.CheckResult, error)
	Reset(ctx context.Context, tenantID string, limitKeys []string) error
	GetUsage(ctx context.Context, tenantID string) ([]models.UsageRecord, error)
	GetUsageByKey(ctx context.Context, tenantID string, limitKey string) (*models.UsageRecord, error)

	// ApplyOperations applies usage operations directly without checking limits.
	// Used by TransactionService to commit pending operations.
	ApplyOperations(ctx context.Context, tenantID string, operations []UsageOperation) error
}

type usageService struct {
	repos  *repositories.Repositories
	cfg    *config.Config
	log    logger.Logger
	engine *limiter.Engine
}

func NewUsageService(repos *repositories.Repositories, cfg *config.Config, log logger.Logger, engine *limiter.Engine) UsageService {
	return &usageService{repos: repos, cfg: cfg, log: log, engine: engine}
}

func (s *usageService) Check(ctx context.Context, tenantID string, operations []UsageOperation, txCtx *TxContext) ([]limiter.CheckResult, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", tenantID)
	}

	if tenant.Status == models.TenantStatusBlocked {
		return nil, errors.TenantBlocked(tenantID)
	}

	if tenant.Status == models.TenantStatusSuspended {
		return suspendedResults(operations), nil
	}

	now := time.Now().UTC()

	if txCtx != nil {
		limitKeys := extractLimitKeys(operations)
		txn, err := s.ensureTransaction(ctx, txCtx, tenantID, limitKeys)
		if err != nil {
			return nil, err
		}
		return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, pendingAdjustments(txn))
	}

	return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, nil)
}

func (s *usageService) Increment(ctx context.Context, tenantID string, operations []UsageOperation, txCtx *TxContext) ([]limiter.CheckResult, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", tenantID)
	}

	if tenant.Status == models.TenantStatusBlocked {
		return nil, errors.TenantBlocked(tenantID)
	}

	if tenant.Status == models.TenantStatusSuspended {
		return suspendedResults(operations), nil
	}

	now := time.Now().UTC()
	limitKeys := extractLimitKeys(operations)

	if txCtx != nil {
		return s.incrementTransactional(ctx, tenant, operations, txCtx, limitKeys, now)
	}

	// Non-transactional: check locks first.
	if err := s.checkLocks(ctx, tenantID, limitKeys); err != nil {
		return nil, err
	}

	// Phase 1: Check all limits (own + ancestor global) first.
	results, err := s.checkLimitsWithHierarchy(ctx, tenant, operations, now, nil)
	if err != nil {
		return nil, err
	}

	if hasLimitFailure(results) {
		return nil, buildLimitExceededError(tenantID, results)
	}

	// Phase 2: Apply all increments (own + ancestor global).
	if err := s.applyIncrementWithHierarchy(ctx, tenant, operations, now); err != nil {
		return nil, err
	}

	// Re-fetch and return updated results.
	return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, nil)
}

func (s *usageService) incrementTransactional(ctx context.Context, tenant *models.Tenant, operations []UsageOperation, txCtx *TxContext, limitKeys []string, now time.Time) ([]limiter.CheckResult, error) {
	txn, err := s.ensureTransaction(ctx, txCtx, tenant.ID, limitKeys)
	if err != nil {
		return nil, err
	}

	// Speculative check: current usage + pending ops from this tx.
	adj := pendingAdjustments(txn)
	results, err := s.checkLimitsWithHierarchy(ctx, tenant, operations, now, adj)
	if err != nil {
		return nil, err
	}

	if hasLimitFailure(results) {
		return nil, buildLimitExceededError(tenant.ID, results)
	}

	// Record operations in the transaction (deferred — not applied yet).
	for _, op := range operations {
		if err := s.repos.Transactions.AddOperation(ctx, txn.ID, models.TransactionOp{
			LimitKey:  op.LimitKey,
			Operation: "increment",
			Amount:    op.Amount,
		}); err != nil {
			return nil, err
		}
		adj[op.LimitKey] += op.Amount
	}

	// Return speculative post-increment state.
	return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, adj)
}

func (s *usageService) Decrement(ctx context.Context, tenantID string, operations []UsageOperation, txCtx *TxContext) ([]limiter.CheckResult, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", tenantID)
	}

	if tenant.Status == models.TenantStatusBlocked {
		return nil, errors.TenantBlocked(tenantID)
	}

	if tenant.Status == models.TenantStatusSuspended {
		return suspendedResults(operations), nil
	}

	now := time.Now().UTC()
	limitKeys := extractLimitKeys(operations)

	if txCtx != nil {
		return s.decrementTransactional(ctx, tenant, operations, txCtx, limitKeys, now)
	}

	// Non-transactional: check locks first.
	if err := s.checkLocks(ctx, tenantID, limitKeys); err != nil {
		return nil, err
	}

	// Check below-zero constraint if configured (only for the tenant's own usage).
	if !s.cfg.AllowUsageBelowZero {
		for _, op := range operations {
			usage, err := s.repos.Usage.GetUsage(ctx, tenantID, op.LimitKey)
			if err != nil {
				return nil, err
			}
			current := int64(0)
			if usage != nil {
				current = usage.CurrentValue
			}
			if current-op.Amount < 0 {
				return nil, errors.BelowZero(tenantID, op.LimitKey)
			}
		}
	}

	// Apply decrements (own + ancestor global).
	negOps := make([]UsageOperation, len(operations))
	for i, op := range operations {
		negOps[i] = UsageOperation{LimitKey: op.LimitKey, Amount: -op.Amount}
	}
	if err := s.applyIncrementWithHierarchy(ctx, tenant, negOps, now); err != nil {
		return nil, err
	}

	return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, nil)
}

func (s *usageService) decrementTransactional(ctx context.Context, tenant *models.Tenant, operations []UsageOperation, txCtx *TxContext, limitKeys []string, now time.Time) ([]limiter.CheckResult, error) {
	txn, err := s.ensureTransaction(ctx, txCtx, tenant.ID, limitKeys)
	if err != nil {
		return nil, err
	}

	adj := pendingAdjustments(txn)

	// Speculative below-zero check.
	if !s.cfg.AllowUsageBelowZero {
		for _, op := range operations {
			usage, err := s.repos.Usage.GetUsage(ctx, tenant.ID, op.LimitKey)
			if err != nil {
				return nil, err
			}
			current := int64(0)
			if usage != nil {
				current = usage.CurrentValue
			}
			current += adj[op.LimitKey]
			if current-op.Amount < 0 {
				return nil, errors.BelowZero(tenant.ID, op.LimitKey)
			}
		}
	}

	// Record decrement operations.
	for _, op := range operations {
		if err := s.repos.Transactions.AddOperation(ctx, txn.ID, models.TransactionOp{
			LimitKey:  op.LimitKey,
			Operation: "decrement",
			Amount:    op.Amount,
		}); err != nil {
			return nil, err
		}
		adj[op.LimitKey] -= op.Amount
	}

	// Return speculative post-decrement state.
	return s.checkLimitsWithHierarchy(ctx, tenant, operations, now, adj)
}

func (s *usageService) Reset(ctx context.Context, tenantID string, limitKeys []string) error {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.NotFound("tenant", tenantID)
	}

	if len(limitKeys) == 0 {
		return s.repos.Usage.ResetAllUsage(ctx, tenantID)
	}

	for _, key := range limitKeys {
		if err := s.repos.Usage.ResetUsage(ctx, tenantID, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *usageService) GetUsage(ctx context.Context, tenantID string) ([]models.UsageRecord, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", tenantID)
	}
	return s.repos.Usage.GetAllUsage(ctx, tenantID)
}

func (s *usageService) GetUsageByKey(ctx context.Context, tenantID string, limitKey string) (*models.UsageRecord, error) {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errors.NotFound("tenant", tenantID)
	}
	return s.repos.Usage.GetUsage(ctx, tenantID, limitKey)
}

func (s *usageService) ApplyOperations(ctx context.Context, tenantID string, operations []UsageOperation) error {
	tenant, err := s.repos.Tenants.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.NotFound("tenant", tenantID)
	}
	return s.applyIncrementWithHierarchy(ctx, tenant, operations, time.Now().UTC())
}

// --- Transaction Helpers ---

// ensureTransaction creates a new transaction (implicit begin) or joins an existing one.
// It locks the given limit keys, checking for conflicts with other transactions.
func (s *usageService) ensureTransaction(ctx context.Context, txCtx *TxContext, tenantID string, limitKeys []string) (*models.Transaction, error) {
	now := time.Now().UTC()

	tx, err := s.repos.Transactions.GetByID(ctx, txCtx.TransactionID)
	if err != nil {
		return nil, err
	}

	if tx != nil {
		// Existing transaction — validate.
		if tx.Status != models.TransactionStatusPending {
			return nil, errors.BadRequest("transaction " + tx.ID + " is not pending (status: " + string(tx.Status) + ")")
		}
		if tx.TenantID != tenantID {
			return nil, errors.BadRequest("transaction " + tx.ID + " belongs to a different tenant")
		}
		if tx.ExpiresAt != nil && now.After(*tx.ExpiresAt) {
			s.repos.Transactions.UpdateStatus(ctx, tx.ID, models.TransactionStatusExpired)
			return nil, errors.BadRequest("transaction " + tx.ID + " has expired")
		}

		newKeys := findNewKeys(tx.LimitKeys, limitKeys)
		if len(newKeys) > 0 {
			for _, k := range newKeys {
				existing, err := s.repos.Transactions.GetPendingByTenantAndLimitKey(ctx, tenantID, k)
				if err != nil {
					return nil, err
				}
				if existing != nil && existing.ID != tx.ID {
					return nil, errors.TransactionConflict(existing.ID, tenantID, k)
				}
			}
			if err := s.repos.Transactions.AddLimitKeys(ctx, tx.ID, newKeys); err != nil {
				return nil, err
			}
			tx.LimitKeys = append(tx.LimitKeys, newKeys...)
		}
		return tx, nil
	}

	// New transaction — implicit begin.
	for _, k := range limitKeys {
		existing, err := s.repos.Transactions.GetPendingByTenantAndLimitKey(ctx, tenantID, k)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, errors.TransactionConflict(existing.ID, tenantID, k)
		}
	}

	tx = &models.Transaction{
		ID:        txCtx.TransactionID,
		TenantID:  tenantID,
		LimitKeys: limitKeys,
		Status:    models.TransactionStatusPending,
		TTL:       txCtx.TTL,
		CreatedAt: now,
	}
	if txCtx.TTL != nil {
		exp := now.Add(*txCtx.TTL)
		tx.ExpiresAt = &exp
	}

	if err := s.repos.Transactions.Create(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// checkLocks verifies no pending transaction holds a lock on the given (tenant, limitKey) tuples.
func (s *usageService) checkLocks(ctx context.Context, tenantID string, limitKeys []string) error {
	now := time.Now().UTC()
	for _, k := range limitKeys {
		existing, err := s.repos.Transactions.GetPendingByTenantAndLimitKey(ctx, tenantID, k)
		if err != nil {
			return err
		}
		if existing != nil {
			// Auto-expire if TTL exceeded.
			if existing.ExpiresAt != nil && now.After(*existing.ExpiresAt) {
				s.repos.Transactions.UpdateStatus(ctx, existing.ID, models.TransactionStatusExpired)
				continue
			}
			return errors.TransactionConflict(existing.ID, tenantID, k)
		}
	}
	return nil
}

// pendingAdjustments computes net usage adjustments from a transaction's pending operations.
func pendingAdjustments(tx *models.Transaction) map[string]int64 {
	adj := make(map[string]int64)
	for _, op := range tx.Operations {
		switch op.Operation {
		case "increment":
			adj[op.LimitKey] += op.Amount
		case "decrement":
			adj[op.LimitKey] -= op.Amount
		}
	}
	return adj
}

func findNewKeys(existing, requested []string) []string {
	set := make(map[string]bool, len(existing))
	for _, k := range existing {
		set[k] = true
	}
	var result []string
	for _, k := range requested {
		if !set[k] {
			result = append(result, k)
		}
	}
	return result
}

// adjustUsage returns a copy of the usage record with pending transaction adjustments applied.
func adjustUsage(usage *models.UsageRecord, limitKey string, pendingAdj map[string]int64) *models.UsageRecord {
	if pendingAdj == nil {
		return usage
	}
	adj := pendingAdj[limitKey]
	if adj == 0 {
		return usage
	}
	if usage == nil {
		return &models.UsageRecord{CurrentValue: adj}
	}
	u := *usage
	u.CurrentValue += adj
	return &u
}

// --- Hierarchy Helpers ---

// ancestorGlobalLimit describes a global limit found on an ancestor tenant.
type ancestorGlobalLimit struct {
	ancestor   *models.Tenant
	definition *models.LimitDefinition
}

// getAncestorGlobalLimits returns, for each operation key, the list of ancestor
// global limits that match. This walks up the ancestor chain and collects global
// limit definitions with matching keys.
func (s *usageService) getAncestorGlobalLimits(ctx context.Context, tenantID string, limitKeys []string) (map[string][]ancestorGlobalLimit, error) {
	ancestors, err := s.repos.Tenants.GetAncestors(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	keySet := make(map[string]bool, len(limitKeys))
	for _, k := range limitKeys {
		keySet[k] = true
	}

	result := make(map[string][]ancestorGlobalLimit)
	for i := range ancestors {
		anc := &ancestors[i]
		if anc.TierID == nil {
			continue
		}

		limits, err := s.repos.LimitDefinitions.GetByTier(ctx, *anc.TierID)
		if err != nil {
			return nil, err
		}

		for j := range limits {
			def := &limits[j]
			if def.Scope == models.LimitScopeGlobal && keySet[def.Key] {
				result[def.Key] = append(result[def.Key], ancestorGlobalLimit{
					ancestor:   anc,
					definition: def,
				})
			}
		}
	}

	return result, nil
}

// checkLimitsWithHierarchy evaluates operations against the tenant's own limits
// AND all ancestor global limits with matching keys. pendingAdj adds speculative
// adjustments for transaction operations not yet committed.
func (s *usageService) checkLimitsWithHierarchy(ctx context.Context, tenant *models.Tenant, operations []UsageOperation, now time.Time, pendingAdj map[string]int64) ([]limiter.CheckResult, error) {
	// Collect limit keys for ancestor lookup.
	limitKeys := make([]string, len(operations))
	for i, op := range operations {
		limitKeys[i] = op.LimitKey
	}

	// Get ancestor global limits.
	ancestorLimits, err := s.getAncestorGlobalLimits(ctx, tenant.ID, limitKeys)
	if err != nil {
		return nil, err
	}

	// Build tenant's own limit map (nil if tier-less).
	var ownLimitMap map[string]*models.LimitDefinition
	if tenant.TierID != nil {
		limits, err := s.repos.LimitDefinitions.GetByTier(ctx, *tenant.TierID)
		if err != nil {
			return nil, err
		}
		ownLimitMap = make(map[string]*models.LimitDefinition, len(limits))
		for i := range limits {
			ownLimitMap[limits[i].Key] = &limits[i]
		}
	}

	results := make([]limiter.CheckResult, len(operations))
	for i, op := range operations {
		// Start with the tenant's own check.
		var ownResult *limiter.CheckResult
		if ownLimitMap != nil {
			def, ok := ownLimitMap[op.LimitKey]
			if ok {
				usage, err := s.repos.Usage.GetUsage(ctx, tenant.ID, op.LimitKey)
				if err != nil {
					return nil, err
				}
				usage = adjustUsage(usage, op.LimitKey, pendingAdj)
				ownResult, err = s.engine.Check(def, usage, op.Amount, now)
				if err != nil {
					return nil, err
				}
			}
		}

		// If no own limit, start with an "allowed" result.
		if ownResult == nil {
			ownResult = &limiter.CheckResult{
				Allowed:   true,
				LimitKey:  op.LimitKey,
				LimitKind: "none",
				Remaining: -1,
			}
		}

		// Check ancestor global limits.
		mostRestrictive := ownResult
		for _, agl := range ancestorLimits[op.LimitKey] {
			usage, err := s.repos.Usage.GetUsage(ctx, agl.ancestor.ID, op.LimitKey)
			if err != nil {
				return nil, err
			}
			usage = adjustUsage(usage, op.LimitKey, pendingAdj)
			ancResult, err := s.engine.Check(agl.definition, usage, op.Amount, now)
			if err != nil {
				return nil, err
			}
			// If ancestor check fails, the overall result fails.
			if !ancResult.Allowed {
				mostRestrictive = ancResult
				break
			}
			// Track the most restrictive remaining value.
			if mostRestrictive.Remaining == -1 || (ancResult.Remaining >= 0 && ancResult.Remaining < mostRestrictive.Remaining) {
				if mostRestrictive == ownResult && mostRestrictive.Allowed {
					merged := *ownResult
					merged.Remaining = ancResult.Remaining
					mostRestrictive = &merged
				}
			}
		}

		results[i] = *mostRestrictive
	}

	return results, nil
}

// applyIncrementWithHierarchy applies usage increments for the tenant and all
// ancestors that have matching global limits.
func (s *usageService) applyIncrementWithHierarchy(ctx context.Context, tenant *models.Tenant, operations []UsageOperation, now time.Time) error {
	limitKeys := make([]string, len(operations))
	for i, op := range operations {
		limitKeys[i] = op.LimitKey
	}

	ancestorLimits, err := s.getAncestorGlobalLimits(ctx, tenant.ID, limitKeys)
	if err != nil {
		return err
	}

	for _, op := range operations {
		// Apply to the tenant's own usage (only if they have a tier).
		if tenant.TierID != nil {
			if err := s.applyIncrementSingle(ctx, tenant.ID, op.LimitKey, op.Amount, now, tenant.TierID); err != nil {
				return err
			}
		}

		// Apply to each ancestor that has a matching global limit.
		for _, agl := range ancestorLimits[op.LimitKey] {
			if err := s.applyIncrementSingle(ctx, agl.ancestor.ID, op.LimitKey, op.Amount, now, agl.ancestor.TierID); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyIncrementSingle updates a single usage record for one (tenant, limitKey) pair.
func (s *usageService) applyIncrementSingle(ctx context.Context, tenantID, limitKey string, amount int64, now time.Time, tierID *string) error {
	usage, err := s.repos.Usage.GetUsage(ctx, tenantID, limitKey)
	if err != nil {
		return err
	}

	if usage == nil {
		usage = &models.UsageRecord{
			ID:       models.NewID(),
			TenantID: tenantID,
			LimitKey: limitKey,
		}
	}

	// Handle time-based window reset.
	if tierID != nil {
		def, err := s.repos.LimitDefinitions.GetByTierAndKey(ctx, *tierID, limitKey)
		if err != nil {
			return err
		}
		if def != nil && def.Kind == models.LimitKindTimeBased {
			cfg := def.Config.(models.TimeBasedConfig)
			if usage.WindowEnd != nil && now.After(*usage.WindowEnd) {
				usage.CurrentValue = 0
				start, end := limiter.WindowForUsage(cfg, now)
				usage.WindowStart = &start
				usage.WindowEnd = &end
			} else if usage.WindowStart == nil {
				start, end := limiter.WindowForUsage(cfg, now)
				usage.WindowStart = &start
				usage.WindowEnd = &end
			}
		}
	}

	usage.CurrentValue += amount
	usage.UpdatedAt = now

	return s.repos.Usage.UpsertUsage(ctx, usage)
}

// --- Shared Helpers ---

func extractLimitKeys(operations []UsageOperation) []string {
	keys := make([]string, len(operations))
	for i, op := range operations {
		keys[i] = op.LimitKey
	}
	return keys
}

func suspendedResults(operations []UsageOperation) []limiter.CheckResult {
	results := make([]limiter.CheckResult, len(operations))
	for i, op := range operations {
		results[i] = limiter.CheckResult{
			Allowed:   true,
			LimitKey:  op.LimitKey,
			LimitKind: "suspended",
			Remaining: -1,
		}
	}
	return results
}

func hasLimitFailure(results []limiter.CheckResult) bool {
	for _, r := range results {
		if !r.Allowed {
			return true
		}
	}
	return false
}

func buildLimitExceededError(tenantID string, results []limiter.CheckResult) error {
	allDetails := make([]errors.LimitDetail, len(results))
	for i, r := range results {
		allDetails[i] = errors.LimitDetail{
			LimitKey:     r.LimitKey,
			LimitKind:    r.LimitKind,
			Exceeded:     !r.Allowed,
			CurrentValue: r.CurrentValue,
			LimitValue:   r.LimitValue,
			Remaining:    r.Remaining,
		}
		if r.ResetAt != nil {
			ts := r.ResetAt.Format(time.RFC3339)
			allDetails[i].ResetAt = &ts
		}
		allDetails[i].RetryAfterSecs = r.RetryAfterSecs
	}
	return errors.LimitExceeded(tenantID, allDetails)
}
