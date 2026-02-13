package services

import (
	"context"
	"testing"
	"time"

	"tierify/internal/config"
	"tierify/internal/errors"
	"tierify/internal/limiter"
	"tierify/internal/logger"
	"tierify/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helpers ---

type txTestEnv struct {
	usageSvc  UsageService
	txSvc     TransactionService
	tierSvc   TierService
	tenantSvc TenantService
	txRepo    *mockTransactionRepo
	usageRepo *mockUsageRepo
}

func newTxTestEnv(cfg *config.Config) *txTestEnv {
	repos, _, _, usageRepo, txRepo := newMockRepos()
	if cfg == nil {
		cfg = &config.Config{}
	}
	log := logger.Nop()
	engine := limiter.NewEngine()
	tierSvc := NewTierService(repos, cfg, log)
	tenantSvc := NewTenantService(repos, cfg, log)
	usageSvc := NewUsageService(repos, cfg, log, engine)
	txSvc := NewTransactionService(repos, usageSvc, log)
	return &txTestEnv{
		usageSvc:  usageSvc,
		txSvc:     txSvc,
		tierSvc:   tierSvc,
		tenantSvc: tenantSvc,
		txRepo:    txRepo,
		usageRepo: usageRepo,
	}
}

func (e *txTestEnv) setupTierAndTenant(t *testing.T) (*models.Tier, *models.Tenant) {
	t.Helper()
	ctx := context.Background()
	limits := []models.LimitDefinition{
		{Key: "apiCalls", Kind: models.LimitKindAbsolute, Scope: models.LimitScopeLocal, Config: models.AbsoluteConfig{MaxValue: 100}},
		{Key: "storage", Kind: models.LimitKindCumulative, Scope: models.LimitScopeLocal, Config: models.CumulativeConfig{MaxValue: 1000, Unit: "bytes"}},
	}
	tier, err := e.tierSvc.Create(ctx, "pro", "Pro", limits)
	require.NoError(t, err)

	tierKey := "pro"
	tenant, err := e.tenantSvc.Create(ctx, "user-1", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	return tier, tenant
}

// --- TransactionService Unit Tests ---

func TestTransactionService_Get_Found(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Implicitly create a transaction via a transactional check.
	txCtx := &TxContext{TransactionID: "tx-1"}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Get the transaction.
	tx, err := env.txSvc.Get(ctx, "tx-1")
	require.NoError(t, err)
	assert.Equal(t, "tx-1", tx.ID)
	assert.Equal(t, tenant.ID, tx.TenantID)
	assert.Equal(t, models.TransactionStatusPending, tx.Status)
}

func TestTransactionService_Get_NotFound(t *testing.T) {
	env := newTxTestEnv(nil)
	ctx := context.Background()

	_, err := env.txSvc.Get(ctx, "nonexistent")
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "not_found", appErr.Code)
}

func TestTransactionService_Get_AutoExpire(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create a transaction with a very short TTL.
	ttl := time.Millisecond
	txCtx := &TxContext{TransactionID: "tx-expire", TTL: &ttl}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	// Get should auto-expire the transaction.
	tx, err := env.txSvc.Get(ctx, "tx-expire")
	require.NoError(t, err)
	assert.Equal(t, models.TransactionStatusExpired, tx.Status)
}

func TestTransactionService_Commit_Success(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create transaction and add operations.
	txCtx := &TxContext{TransactionID: "tx-commit"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, txCtx)
	require.NoError(t, err)

	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "storage", Amount: 200},
	}, txCtx)
	require.NoError(t, err)

	// Verify usage is NOT applied yet.
	usage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	if usage != nil {
		assert.Equal(t, int64(0), usage.CurrentValue)
	}

	// Commit the transaction.
	err = env.txSvc.Commit(ctx, "tx-commit")
	require.NoError(t, err)

	// Verify usage IS applied after commit.
	apiUsage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, apiUsage)
	assert.Equal(t, int64(10), apiUsage.CurrentValue)

	storageUsage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "storage")
	require.NoError(t, err)
	require.NotNil(t, storageUsage)
	assert.Equal(t, int64(200), storageUsage.CurrentValue)

	// Verify transaction status.
	tx, err := env.txSvc.Get(ctx, "tx-commit")
	require.NoError(t, err)
	assert.Equal(t, models.TransactionStatusCommitted, tx.Status)
}

func TestTransactionService_Commit_WithDecrements(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// First set up some usage.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, nil)
	require.NoError(t, err)

	// Create transaction with a decrement.
	txCtx := &TxContext{TransactionID: "tx-dec"}
	_, err = env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, txCtx)
	require.NoError(t, err)

	// Usage should still be 50 (not yet committed).
	usage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(50), usage.CurrentValue)

	// Commit.
	err = env.txSvc.Commit(ctx, "tx-dec")
	require.NoError(t, err)

	// Usage should now be 30 (50 - 20).
	usage, err = env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(30), usage.CurrentValue)
}

func TestTransactionService_Commit_NotPending(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create and commit a transaction.
	txCtx := &TxContext{TransactionID: "tx-done"}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)
	err = env.txSvc.Commit(ctx, "tx-done")
	require.NoError(t, err)

	// Try to commit again.
	err = env.txSvc.Commit(ctx, "tx-done")
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "bad_request", appErr.Code)
}

func TestTransactionService_Rollback_Success(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create transaction with operations.
	txCtx := &TxContext{TransactionID: "tx-rollback"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, txCtx)
	require.NoError(t, err)

	// Rollback.
	err = env.txSvc.Rollback(ctx, "tx-rollback")
	require.NoError(t, err)

	// Verify status.
	tx, err := env.txSvc.Get(ctx, "tx-rollback")
	require.NoError(t, err)
	assert.Equal(t, models.TransactionStatusRolledBack, tx.Status)

	// Verify usage was never applied.
	usage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	if usage != nil {
		assert.Equal(t, int64(0), usage.CurrentValue)
	}
}

func TestTransactionService_Rollback_NotPending(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create and rollback.
	txCtx := &TxContext{TransactionID: "tx-rb"}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)
	err = env.txSvc.Rollback(ctx, "tx-rb")
	require.NoError(t, err)

	// Try again.
	err = env.txSvc.Rollback(ctx, "tx-rb")
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "bad_request", appErr.Code)
}

// --- Transactional Usage Tests ---

func TestTransaction_ImplicitBegin_ViaCheck(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-begin"}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Transaction should exist now.
	tx, err := env.txSvc.Get(ctx, "tx-begin")
	require.NoError(t, err)
	assert.Equal(t, models.TransactionStatusPending, tx.Status)
	assert.Equal(t, tenant.ID, tx.TenantID)
	assert.Contains(t, tx.LimitKeys, "apiCalls")
}

func TestTransaction_ImplicitBegin_ViaIncrement(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-inc"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	tx, err := env.txSvc.Get(ctx, "tx-inc")
	require.NoError(t, err)
	assert.Equal(t, models.TransactionStatusPending, tx.Status)
	assert.Len(t, tx.Operations, 1)
	assert.Equal(t, "increment", tx.Operations[0].Operation)
	assert.Equal(t, int64(5), tx.Operations[0].Amount)
}

func TestTransaction_ImplicitBegin_ViaDecrement(t *testing.T) {
	env := newTxTestEnv(&config.Config{AllowUsageBelowZero: true})
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-dec"}
	_, err := env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 3},
	}, txCtx)
	require.NoError(t, err)

	tx, err := env.txSvc.Get(ctx, "tx-dec")
	require.NoError(t, err)
	assert.Len(t, tx.Operations, 1)
	assert.Equal(t, "decrement", tx.Operations[0].Operation)
	assert.Equal(t, int64(3), tx.Operations[0].Amount)
}

func TestTransaction_ImplicitBegin_WithTTL(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	ttl := 30 * time.Second
	txCtx := &TxContext{TransactionID: "tx-ttl", TTL: &ttl}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	tx, err := env.txSvc.Get(ctx, "tx-ttl")
	require.NoError(t, err)
	assert.NotNil(t, tx.ExpiresAt)
	assert.NotNil(t, tx.TTL)
	assert.Equal(t, 30*time.Second, *tx.TTL)
}

func TestTransaction_JoinExisting(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-join"}

	// First operation creates the transaction.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	// Second operation on a new key joins the same transaction.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "storage", Amount: 100},
	}, txCtx)
	require.NoError(t, err)

	tx, err := env.txSvc.Get(ctx, "tx-join")
	require.NoError(t, err)
	assert.Len(t, tx.Operations, 2)
	assert.Contains(t, tx.LimitKeys, "apiCalls")
	assert.Contains(t, tx.LimitKeys, "storage")
}

func TestTransaction_LockConflict_DifferentTransaction(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// First transaction locks apiCalls.
	txCtx1 := &TxContext{TransactionID: "tx-1"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx1)
	require.NoError(t, err)

	// Second transaction tries to lock same key.
	txCtx2 := &TxContext{TransactionID: "tx-2"}
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 3},
	}, txCtx2)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "transaction_conflict", appErr.Code)
}

func TestTransaction_LockConflict_NonTransactionalBlocked(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Transaction locks apiCalls.
	txCtx := &TxContext{TransactionID: "tx-lock"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	// Non-transactional increment on same key should be blocked.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "transaction_conflict", appErr.Code)
}

func TestTransaction_LockConflict_NonTransactionalDecrement(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Set up some usage first.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, nil)
	require.NoError(t, err)

	// Transaction locks apiCalls.
	txCtx := &TxContext{TransactionID: "tx-lock"}
	_, err = env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Non-transactional decrement should be blocked.
	_, err = env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, nil)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "transaction_conflict", appErr.Code)
}

func TestTransaction_NoLockConflict_DifferentKeys(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Transaction locks apiCalls.
	txCtx := &TxContext{TransactionID: "tx-api"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	// Non-transactional increment on different key is fine.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "storage", Amount: 100},
	}, nil)
	require.NoError(t, err)
}

func TestTransaction_NoLockConflict_CheckIsReadOnly(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Transaction locks apiCalls.
	txCtx := &TxContext{TransactionID: "tx-lock"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	// Non-transactional CHECK on same key should still work (read-only).
	results, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.NoError(t, err)
	assert.True(t, results[0].Allowed)
}

func TestTransaction_SpeculativeCheck_IncludesPendingOps(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-spec"}

	// Increment 80 in transaction.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 80},
	}, txCtx)
	require.NoError(t, err)

	// Transactional check for 30 more — should fail (80+30=110 > 100).
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 30},
	}, txCtx)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "limit_exceeded", appErr.Code)
}

func TestTransaction_SpeculativeCheck_WithinLimit(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	txCtx := &TxContext{TransactionID: "tx-spec2"}

	// Increment 50 in transaction.
	results, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, txCtx)
	require.NoError(t, err)
	// Speculative result should show current value as 50.
	assert.Equal(t, int64(50), results[0].CurrentValue)

	// Increment 30 more (total 80, within limit of 100). Succeeds (no error).
	results, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 30},
	}, txCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(80), results[0].CurrentValue)
	// Allowed=false because repeating +30 from 80 would exceed 100.
	assert.False(t, results[0].Allowed)
}

func TestTransaction_SpeculativeDecrement_BelowZero(t *testing.T) {
	env := newTxTestEnv(&config.Config{AllowUsageBelowZero: false})
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Set initial usage to 30.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 30},
	}, nil)
	require.NoError(t, err)

	txCtx := &TxContext{TransactionID: "tx-below"}

	// Decrement 20 in transaction (30-20=10, ok).
	_, err = env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, txCtx)
	require.NoError(t, err)

	// Decrement 20 more (speculative: 30-20-20=-10, below zero).
	_, err = env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 20},
	}, txCtx)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "usage_below_zero", appErr.Code)
}

func TestTransaction_WrongTenant(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create a second tenant.
	tierKey := "pro"
	tenant2, err := env.tenantSvc.Create(ctx, "user-2", nil, &tierKey, nil, nil)
	require.NoError(t, err)

	// Create transaction for tenant1.
	txCtx := &TxContext{TransactionID: "tx-tenant1"}
	_, err = env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Try to use same transaction ID for tenant2.
	_, err = env.usageSvc.Check(ctx, tenant2.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "bad_request", appErr.Code)
	assert.Contains(t, appErr.Message, "different tenant")
}

func TestTransaction_ExpiredTransaction_CannotJoin(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create transaction with very short TTL.
	ttl := time.Millisecond
	txCtx := &TxContext{TransactionID: "tx-expired", TTL: &ttl}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	// Try to add more operations — should fail.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.Error(t, err)
	appErr, ok := err.(*errors.AppError)
	require.True(t, ok)
	assert.Equal(t, "bad_request", appErr.Code)
	assert.Contains(t, appErr.Message, "expired")
}

func TestTransaction_LockReleasedAfterCommit(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create and commit a transaction.
	txCtx := &TxContext{TransactionID: "tx-release"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)
	err = env.txSvc.Commit(ctx, "tx-release")
	require.NoError(t, err)

	// Non-transactional increment should now succeed (lock released).
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.NoError(t, err)
}

func TestTransaction_LockReleasedAfterRollback(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create and rollback a transaction.
	txCtx := &TxContext{TransactionID: "tx-rb"}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)
	err = env.txSvc.Rollback(ctx, "tx-rb")
	require.NoError(t, err)

	// Non-transactional increment should now succeed.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.NoError(t, err)
}

func TestTransaction_FullLifecycle(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// 1. Implicit begin via check.
	txCtx := &TxContext{TransactionID: "tx-lifecycle"}
	results, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 10},
	}, txCtx)
	require.NoError(t, err)
	assert.True(t, results[0].Allowed)

	// 2. Add increment operations.
	results, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 30},
	}, txCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(30), results[0].CurrentValue) // Speculative.

	results, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "storage", Amount: 500},
	}, txCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(500), results[0].CurrentValue) // Speculative.

	// 3. Verify no actual usage yet.
	apiUsage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	if apiUsage != nil {
		assert.Equal(t, int64(0), apiUsage.CurrentValue)
	}

	// 4. Verify transaction has all operations.
	tx, err := env.txSvc.Get(ctx, "tx-lifecycle")
	require.NoError(t, err)
	assert.Len(t, tx.Operations, 2)

	// 5. Commit.
	err = env.txSvc.Commit(ctx, "tx-lifecycle")
	require.NoError(t, err)

	// 6. Verify usage applied.
	apiUsage, err = env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	require.NotNil(t, apiUsage)
	assert.Equal(t, int64(30), apiUsage.CurrentValue)

	storageUsage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "storage")
	require.NoError(t, err)
	require.NotNil(t, storageUsage)
	assert.Equal(t, int64(500), storageUsage.CurrentValue)

	// 7. Transaction is committed — can't use again.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.Error(t, err)
}

func TestTransaction_MixedIncrementDecrement(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Set up initial usage.
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 60},
	}, nil)
	require.NoError(t, err)

	txCtx := &TxContext{TransactionID: "tx-mixed"}

	// Decrement 40 (speculative: 60-40=20).
	_, err = env.usageSvc.Decrement(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 40},
	}, txCtx)
	require.NoError(t, err)

	// Increment 50 (speculative: 60-40+50=70).
	results, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 50},
	}, txCtx)
	require.NoError(t, err)
	assert.Equal(t, int64(70), results[0].CurrentValue)

	// Commit.
	err = env.txSvc.Commit(ctx, "tx-mixed")
	require.NoError(t, err)

	// Actual usage: 60 - 40 + 50 = 70.
	usage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	assert.Equal(t, int64(70), usage.CurrentValue)
}

func TestTransaction_Commit_NoOperations(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create transaction with only a check (no increment/decrement).
	txCtx := &TxContext{TransactionID: "tx-noop"}
	_, err := env.usageSvc.Check(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, txCtx)
	require.NoError(t, err)

	// Commit with no operations should succeed.
	err = env.txSvc.Commit(ctx, "tx-noop")
	require.NoError(t, err)

	// No usage should be recorded.
	usage, err := env.usageSvc.GetUsageByKey(ctx, tenant.ID, "apiCalls")
	require.NoError(t, err)
	if usage != nil {
		assert.Equal(t, int64(0), usage.CurrentValue)
	}
}

func TestTransaction_ExpiredLock_NonTransactionalCanProceed(t *testing.T) {
	env := newTxTestEnv(nil)
	_, tenant := env.setupTierAndTenant(t)
	ctx := context.Background()

	// Create transaction with very short TTL.
	ttl := time.Millisecond
	txCtx := &TxContext{TransactionID: "tx-expire-lock", TTL: &ttl}
	_, err := env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 5},
	}, txCtx)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	// Non-transactional increment should succeed because the lock expired.
	_, err = env.usageSvc.Increment(ctx, tenant.ID, []UsageOperation{
		{LimitKey: "apiCalls", Amount: 1},
	}, nil)
	require.NoError(t, err)
}
