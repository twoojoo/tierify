CREATE TABLE IF NOT EXISTS tiers (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    version INTEGER NOT NULL,
    deprecated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(key, version)
);

CREATE INDEX idx_tiers_key ON tiers(key);
CREATE INDEX idx_tiers_key_version ON tiers(key, version);

CREATE TABLE IF NOT EXISTS limit_definitions (
    id TEXT PRIMARY KEY,
    tier_id TEXT NOT NULL REFERENCES tiers(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    kind TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'local',
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(tier_id, key)
);

CREATE INDEX idx_limit_definitions_tier_id ON limit_definitions(tier_id);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    external_id TEXT NOT NULL UNIQUE,
    parent_id TEXT REFERENCES tenants(id),
    tier_id TEXT REFERENCES tiers(id),
    tier_version INTEGER,
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_tenants_external_id ON tenants(external_id);
CREATE INDEX idx_tenants_parent_id ON tenants(parent_id);
CREATE INDEX idx_tenants_tier_id ON tenants(tier_id);

CREATE TABLE IF NOT EXISTS tenant_tier_history (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    previous_tier_id TEXT,
    previous_version INTEGER,
    new_tier_id TEXT,
    new_version INTEGER,
    usage_reset BOOLEAN NOT NULL DEFAULT FALSE,
    changed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_tenant_tier_history_tenant_id ON tenant_tier_history(tenant_id);

CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    limit_key TEXT NOT NULL,
    current_value BIGINT NOT NULL DEFAULT 0,
    window_start TIMESTAMPTZ,
    window_end TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, limit_key)
);

CREATE INDEX idx_usage_records_tenant_id ON usage_records(tenant_id);
CREATE INDEX idx_usage_records_tenant_limit ON usage_records(tenant_id, limit_key);

CREATE TABLE IF NOT EXISTS transactions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    limit_keys JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    ttl_seconds INTEGER,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_transactions_tenant_id ON transactions(tenant_id);
CREATE INDEX idx_transactions_status ON transactions(status);

CREATE TABLE IF NOT EXISTS transaction_operations (
    id BIGSERIAL PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    limit_key TEXT NOT NULL,
    operation TEXT NOT NULL,
    amount BIGINT NOT NULL
);

CREATE INDEX idx_transaction_operations_tx_id ON transaction_operations(transaction_id);
