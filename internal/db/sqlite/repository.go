package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tierify/internal/models"
	"tierify/internal/repositories"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewRepositories(connectionString string) (*repositories.Repositories, error) {
	// Append SQLite pragma parameters, handling both plain paths and URI-style connections.
	sep := "?"
	if strings.Contains(connectionString, "?") {
		sep = "&"
	}
	dsn := connectionString + sep + "_foreign_keys=on&_journal_mode=WAL"

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &repositories.Repositories{
		Tiers:            &tierRepo{db: db},
		LimitDefinitions: &limitDefRepo{db: db},
		Tenants:          &tenantRepo{db: db},
		Usage:            &usageRepo{db: db},
		Transactions:     &transactionRepo{db: db},
	}, nil
}

func runMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	dbDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite3", dbDriver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// --- Tier Repository ---

type tierRepo struct{ db *sql.DB }

func (r *tierRepo) Create(ctx context.Context, tier *models.Tier, limits []models.LimitDefinition) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO tiers (id, key, name, version, deprecated, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tier.ID, tier.Key, tier.Name, tier.Version, boolToInt(tier.Deprecated), tier.CreatedAt, tier.UpdatedAt,
	)
	if err != nil {
		return err
	}

	for _, l := range limits {
		configJSON, err := models.MarshalLimitConfig(l.Config)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO limit_definitions (id, tier_id, key, kind, scope, config, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			l.ID, l.TierID, l.Key, string(l.Kind), string(l.Scope), string(configJSON), l.CreatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *tierRepo) GetByKey(ctx context.Context, key string) (*models.Tier, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, key, name, version, deprecated, created_at, updated_at FROM tiers WHERE key = ? AND deprecated = 0 ORDER BY version DESC LIMIT 1`, key)
	return scanTier(row)
}

func (r *tierRepo) GetByKeyAndVersion(ctx context.Context, key string, version int) (*models.Tier, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, key, name, version, deprecated, created_at, updated_at FROM tiers WHERE key = ? AND version = ?`, key, version)
	return scanTier(row)
}

func (r *tierRepo) ListVersions(ctx context.Context, key string, offset, limit int) (*repositories.PaginatedResult[models.Tier], error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tiers WHERE key = ?`, key).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, key, name, version, deprecated, created_at, updated_at FROM tiers WHERE key = ? ORDER BY version DESC LIMIT ? OFFSET ?`,
		key, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiers, err := scanTiers(rows)
	if err != nil {
		return nil, err
	}

	return &repositories.PaginatedResult[models.Tier]{
		Data:       tiers,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

func (r *tierRepo) List(ctx context.Context, filters models.TierFilters) (*repositories.PaginatedResult[models.Tier], error) {
	where, args := buildTierFilters(filters)

	var total int
	r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tiers `+where, args...).Scan(&total)

	query := `SELECT id, key, name, version, deprecated, created_at, updated_at FROM tiers ` + where + ` ORDER BY key, version DESC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiers, err := scanTiers(rows)
	if err != nil {
		return nil, err
	}

	return &repositories.PaginatedResult[models.Tier]{
		Data:       tiers,
		Pagination: repositories.Pagination{Offset: filters.Offset, Limit: filters.Limit, Total: total},
	}, nil
}

func (r *tierRepo) Deprecate(ctx context.Context, key string, version int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tiers SET deprecated = 1, updated_at = ? WHERE key = ? AND version = ?`,
		time.Now().UTC(), key, version)
	return err
}

// --- LimitDefinition Repository ---

type limitDefRepo struct{ db *sql.DB }

func (r *limitDefRepo) GetByTier(ctx context.Context, tierID string) ([]models.LimitDefinition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tier_id, key, kind, scope, config, created_at FROM limit_definitions WHERE tier_id = ?`, tierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var limits []models.LimitDefinition
	for rows.Next() {
		l, err := scanLimitDef(rows)
		if err != nil {
			return nil, err
		}
		limits = append(limits, *l)
	}
	return limits, rows.Err()
}

func (r *limitDefRepo) GetByTierAndKey(ctx context.Context, tierID, limitKey string) (*models.LimitDefinition, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tier_id, key, kind, scope, config, created_at FROM limit_definitions WHERE tier_id = ? AND key = ?`, tierID, limitKey)

	var l models.LimitDefinition
	var kindStr, scopeStr, configJSON string
	err := row.Scan(&l.ID, &l.TierID, &l.Key, &kindStr, &scopeStr, &configJSON, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	l.Kind = models.LimitKind(kindStr)
	l.Scope = models.LimitScope(scopeStr)
	cfg, err := models.ParseLimitConfig(l.Kind, json.RawMessage(configJSON))
	if err != nil {
		return nil, err
	}
	l.Config = cfg
	return &l, nil
}

// --- Tenant Repository ---

type tenantRepo struct{ db *sql.DB }

func (r *tenantRepo) Create(ctx context.Context, tenant *models.Tenant) error {
	metadataJSON, _ := json.Marshal(tenant.Metadata)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tenants (id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tenant.ID, tenant.ExternalID, tenant.ParentID, tenant.TierID, tenant.TierVersion, string(tenant.Status), string(metadataJSON), tenant.CreatedAt, tenant.UpdatedAt,
	)
	return err
}

func (r *tenantRepo) GetByID(ctx context.Context, id string) (*models.Tenant, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at FROM tenants WHERE id = ?`, id)
	return scanTenant(row)
}

func (r *tenantRepo) GetByExternalID(ctx context.Context, externalID string) (*models.Tenant, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at FROM tenants WHERE external_id = ?`, externalID)
	return scanTenant(row)
}

func (r *tenantRepo) GetChildren(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.Tenant], error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE parent_id = ?`, tenantID).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at FROM tenants WHERE parent_id = ? LIMIT ? OFFSET ?`,
		tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenants, err := scanTenants(rows)
	if err != nil {
		return nil, err
	}

	return &repositories.PaginatedResult[models.Tenant]{
		Data:       tenants,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, nil
}

func (r *tenantRepo) GetAncestors(ctx context.Context, tenantID string) ([]models.Tenant, error) {
	// SQLite supports recursive CTEs.
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT t.* FROM tenants t WHERE t.id = (SELECT parent_id FROM tenants WHERE id = ?)
			UNION ALL
			SELECT t.* FROM tenants t INNER JOIN ancestors a ON t.id = a.parent_id
		)
		SELECT id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at FROM ancestors
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenants(rows)
}

func (r *tenantRepo) ListByTierID(ctx context.Context, tierID string) ([]models.Tenant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, external_id, parent_id, tier_id, tier_version, status, metadata, created_at, updated_at FROM tenants WHERE tier_id = ?`, tierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenants(rows)
}

func (r *tenantRepo) Update(ctx context.Context, tenant *models.Tenant) error {
	metadataJSON, _ := json.Marshal(tenant.Metadata)
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET external_id = ?, parent_id = ?, tier_id = ?, tier_version = ?, status = ?, metadata = ?, updated_at = ? WHERE id = ?`,
		tenant.ExternalID, tenant.ParentID, tenant.TierID, tenant.TierVersion, string(tenant.Status), string(metadataJSON), tenant.UpdatedAt, tenant.ID,
	)
	return err
}

func (r *tenantRepo) Delete(ctx context.Context, tenantID string, mode models.DeleteMode) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if mode == models.DeleteModeOrphan {
		_, err = tx.ExecContext(ctx, `UPDATE tenants SET parent_id = NULL WHERE parent_id = ?`, tenantID)
		if err != nil {
			return err
		}
	} else {
		// Cascade: delete all descendants via recursive CTE.
		_, err = tx.ExecContext(ctx, `
			WITH RECURSIVE descendants AS (
				SELECT id FROM tenants WHERE id = ?
				UNION ALL
				SELECT t.id FROM tenants t INNER JOIN descendants d ON t.parent_id = d.id
			)
			DELETE FROM tenants WHERE id IN (SELECT id FROM descendants)
		`, tenantID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = ?`, tenantID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *tenantRepo) RecordTierChange(ctx context.Context, change *models.TenantTierChange) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tenant_tier_history (id, tenant_id, previous_tier_id, previous_version, new_tier_id, new_version, usage_reset, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		change.ID, change.TenantID, change.PreviousTierID, change.PreviousVersion, change.NewTierID, change.NewVersion, boolToInt(change.UsageReset), change.ChangedAt,
	)
	return err
}

func (r *tenantRepo) GetTierHistory(ctx context.Context, tenantID string, offset, limit int) (*repositories.PaginatedResult[models.TenantTierChange], error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_tier_history WHERE tenant_id = ?`, tenantID).Scan(&total)

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, previous_tier_id, previous_version, new_tier_id, new_version, usage_reset, changed_at FROM tenant_tier_history WHERE tenant_id = ? ORDER BY changed_at DESC LIMIT ? OFFSET ?`,
		tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.TenantTierChange
	for rows.Next() {
		var h models.TenantTierChange
		var usageReset int
		if err := rows.Scan(&h.ID, &h.TenantID, &h.PreviousTierID, &h.PreviousVersion, &h.NewTierID, &h.NewVersion, &usageReset, &h.ChangedAt); err != nil {
			return nil, err
		}
		h.UsageReset = usageReset != 0
		history = append(history, h)
	}

	return &repositories.PaginatedResult[models.TenantTierChange]{
		Data:       history,
		Pagination: repositories.Pagination{Offset: offset, Limit: limit, Total: total},
	}, rows.Err()
}

// --- Usage Repository ---

type usageRepo struct{ db *sql.DB }

func (r *usageRepo) GetUsage(ctx context.Context, tenantID, limitKey string) (*models.UsageRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, limit_key, current_value, window_start, window_end, updated_at FROM usage_records WHERE tenant_id = ? AND limit_key = ?`,
		tenantID, limitKey)

	var u models.UsageRecord
	err := row.Scan(&u.ID, &u.TenantID, &u.LimitKey, &u.CurrentValue, &u.WindowStart, &u.WindowEnd, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *usageRepo) GetAllUsage(ctx context.Context, tenantID string) ([]models.UsageRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, limit_key, current_value, window_start, window_end, updated_at FROM usage_records WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.UsageRecord
	for rows.Next() {
		var u models.UsageRecord
		if err := rows.Scan(&u.ID, &u.TenantID, &u.LimitKey, &u.CurrentValue, &u.WindowStart, &u.WindowEnd, &u.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, u)
	}
	return records, rows.Err()
}

func (r *usageRepo) UpsertUsage(ctx context.Context, record *models.UsageRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO usage_records (id, tenant_id, limit_key, current_value, window_start, window_end, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, limit_key)
		DO UPDATE SET current_value = ?, window_start = ?, window_end = ?, updated_at = ?`,
		record.ID, record.TenantID, record.LimitKey, record.CurrentValue, record.WindowStart, record.WindowEnd, record.UpdatedAt,
		record.CurrentValue, record.WindowStart, record.WindowEnd, record.UpdatedAt,
	)
	return err
}

func (r *usageRepo) ResetUsage(ctx context.Context, tenantID, limitKey string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM usage_records WHERE tenant_id = ? AND limit_key = ?`, tenantID, limitKey)
	return err
}

func (r *usageRepo) ResetAllUsage(ctx context.Context, tenantID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM usage_records WHERE tenant_id = ?`, tenantID)
	return err
}

// --- Transaction Repository ---

type transactionRepo struct{ db *sql.DB }

func (r *transactionRepo) Create(ctx context.Context, txn *models.Transaction) error {
	limitKeysJSON, _ := json.Marshal(txn.LimitKeys)
	var ttlSeconds *int64
	if txn.TTL != nil {
		s := int64(txn.TTL.Seconds())
		ttlSeconds = &s
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transactions (id, tenant_id, limit_keys, status, ttl_seconds, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		txn.ID, txn.TenantID, string(limitKeysJSON), string(txn.Status), ttlSeconds, txn.CreatedAt, txn.ExpiresAt,
	)
	return err
}

func (r *transactionRepo) GetByID(ctx context.Context, id string) (*models.Transaction, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, limit_keys, status, ttl_seconds, created_at, expires_at FROM transactions WHERE id = ?`, id)

	var txn models.Transaction
	var limitKeysJSON string
	var statusStr string
	var ttlSeconds *int64
	err := row.Scan(&txn.ID, &txn.TenantID, &limitKeysJSON, &statusStr, &ttlSeconds, &txn.CreatedAt, &txn.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(limitKeysJSON), &txn.LimitKeys)
	txn.Status = models.TransactionStatus(statusStr)
	if ttlSeconds != nil {
		d := time.Duration(*ttlSeconds) * time.Second
		txn.TTL = &d
	}

	// Load operations.
	ops, err := r.db.QueryContext(ctx,
		`SELECT limit_key, operation, amount FROM transaction_operations WHERE transaction_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer ops.Close()

	for ops.Next() {
		var op models.TransactionOp
		if err := ops.Scan(&op.LimitKey, &op.Operation, &op.Amount); err != nil {
			return nil, err
		}
		txn.Operations = append(txn.Operations, op)
	}

	return &txn, ops.Err()
}

func (r *transactionRepo) AddOperation(ctx context.Context, txID string, op models.TransactionOp) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transaction_operations (transaction_id, limit_key, operation, amount) VALUES (?, ?, ?, ?)`,
		txID, op.LimitKey, op.Operation, op.Amount,
	)
	return err
}

func (r *transactionRepo) AddLimitKeys(ctx context.Context, txID string, keys []string) error {
	txn, err := r.GetByID(ctx, txID)
	if err != nil || txn == nil {
		return err
	}
	existing := make(map[string]bool)
	for _, k := range txn.LimitKeys {
		existing[k] = true
	}
	for _, k := range keys {
		if !existing[k] {
			txn.LimitKeys = append(txn.LimitKeys, k)
		}
	}
	limitKeysJSON, _ := json.Marshal(txn.LimitKeys)
	_, err = r.db.ExecContext(ctx,
		`UPDATE transactions SET limit_keys = ? WHERE id = ?`, string(limitKeysJSON), txID)
	return err
}

func (r *transactionRepo) UpdateStatus(ctx context.Context, txID string, status models.TransactionStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE transactions SET status = ? WHERE id = ?`, string(status), txID)
	return err
}

func (r *transactionRepo) GetPendingByTenantAndLimitKey(ctx context.Context, tenantID, limitKey string) (*models.Transaction, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM transactions WHERE tenant_id = ? AND status = 'pending'`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var txID string
		if err := rows.Scan(&txID); err != nil {
			return nil, err
		}
		txn, err := r.GetByID(ctx, txID)
		if err != nil {
			return nil, err
		}
		for _, k := range txn.LimitKeys {
			if k == limitKey {
				return txn, nil
			}
		}
	}
	return nil, rows.Err()
}

func (r *transactionRepo) CleanExpired(ctx context.Context) (int, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE transactions SET status = 'expired' WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC())
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// --- Helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanTier(row *sql.Row) (*models.Tier, error) {
	var t models.Tier
	var deprecated int
	err := row.Scan(&t.ID, &t.Key, &t.Name, &t.Version, &deprecated, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Deprecated = deprecated != 0
	return &t, nil
}

func scanTiers(rows *sql.Rows) ([]models.Tier, error) {
	var tiers []models.Tier
	for rows.Next() {
		var t models.Tier
		var deprecated int
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.Version, &deprecated, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Deprecated = deprecated != 0
		tiers = append(tiers, t)
	}
	return tiers, rows.Err()
}

func scanLimitDef(rows *sql.Rows) (*models.LimitDefinition, error) {
	var l models.LimitDefinition
	var kindStr, scopeStr, configJSON string
	if err := rows.Scan(&l.ID, &l.TierID, &l.Key, &kindStr, &scopeStr, &configJSON, &l.CreatedAt); err != nil {
		return nil, err
	}
	l.Kind = models.LimitKind(kindStr)
	l.Scope = models.LimitScope(scopeStr)
	cfg, err := models.ParseLimitConfig(l.Kind, json.RawMessage(configJSON))
	if err != nil {
		return nil, err
	}
	l.Config = cfg
	return &l, nil
}

func scanTenant(row *sql.Row) (*models.Tenant, error) {
	var t models.Tenant
	var statusStr string
	var metadataJSON *string
	err := row.Scan(&t.ID, &t.ExternalID, &t.ParentID, &t.TierID, &t.TierVersion, &statusStr, &metadataJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Status = models.TenantStatus(statusStr)
	if metadataJSON != nil {
		json.Unmarshal([]byte(*metadataJSON), &t.Metadata)
	}
	return &t, nil
}

func scanTenants(rows *sql.Rows) ([]models.Tenant, error) {
	var tenants []models.Tenant
	for rows.Next() {
		var t models.Tenant
		var statusStr string
		var metadataJSON *string
		if err := rows.Scan(&t.ID, &t.ExternalID, &t.ParentID, &t.TierID, &t.TierVersion, &statusStr, &metadataJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Status = models.TenantStatus(statusStr)
		if metadataJSON != nil {
			json.Unmarshal([]byte(*metadataJSON), &t.Metadata)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func buildTierFilters(filters models.TierFilters) (string, []any) {
	var conditions []string
	var args []any

	if filters.Key != nil {
		conditions = append(conditions, "key = ?")
		args = append(args, *filters.Key)
	}
	if filters.Deprecated != nil {
		conditions = append(conditions, "deprecated = ?")
		args = append(args, boolToInt(*filters.Deprecated))
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}
