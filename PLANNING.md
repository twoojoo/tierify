# Tierify — Project Planning

## 1. Project Vision

Tierify is a **backend Go service** that acts as a **flexible, composable limit engine** for managing pricing plan tiers. It is designed to be consumed by other backend applications (no end-user auth, no UI) and provides APIs to:

- Define **tiers** with versioning, migration, and deprecation
- Define **limits** of any kind (absolute, time-based, cumulative, burst, compound, feature-based)
- Manage **tenants** in a hierarchical tree (org → department → user, or any shape)
- **Check**, **increment**, **decrement**, and **reset** usage against limits
- Optionally wrap operations in **application-level transactions** for consistency

### Core Principles

- **Flexibility first**: every design decision favors extensibility over simplicity
- **Composable**: limits, tenants, tiers are independent building blocks
- **Database-agnostic**: generic repository interfaces with per-DB implementations
- **Horizontally scalable**: the service itself is completely stateless (no in-process state)
- **No auth/roles**: Tierify is deployed as an internal backend service; the consuming application handles its own auth
- **Test-driven**: near-TDD approach — unit tests for all logic, integration tests with mocked dependencies

---

## 2. Architecture

### 2.1 Layered Structure

```
HTTP Request
    │
    ▼
┌──────────┐
│ Handlers │  ← HTTP routing, request parsing, response formatting
└────┬─────┘
     │
     ▼
┌──────────┐
│ Services │  ← Business logic, limit evaluation, transaction orchestration
└────┬─────┘
     │
     ▼
┌──────────────┐
│ Repositories │  ← Data access via generic interfaces
└────┬─────────┘
     │
     ▼
┌────────────────────────────────┐
│ DB Implementations             │
│ (SQLite, PostgreSQL, MongoDB)  │
└────────────────────────────────┘
```

### 2.2 Tech Stack

| Concern | Choice | Rationale |
|---|---|---|
| HTTP router | `go-chi/chi/v5` | Lightweight, stdlib-compatible, middleware-friendly |
| ID generation | ULID (`oklog/ulid`) | Sortable, DB-index friendly, lexicographic ordering |
| Configuration | `github.com/caarlos0/env/v11` | Env vars validated at startup, collected into a config struct injected everywhere |
| Logging | `log/slog` (stdlib) behind a `Logger` interface | Swappable implementation; slog for now, can switch to zerolog/zap later |
| SQL migrations | `golang-migrate/migrate` | Supports SQLite + Postgres, CLI + programmatic usage, widely adopted |
| Testing | `testing` + `testify` for assertions, interface mocks | Unit tests for all logic, integration tests with real SQLite |

### 2.3 Proposed Project Layout

```
tierify/
├── cmd/
│   └── tierify/
│       └── main.go                 # Entry point
├── internal/
│   ├── config/                     # Configuration struct + env loading
│   │   └── config.go
│   ├── handlers/                   # HTTP handlers (one file per domain)
│   │   ├── tiers.go
│   │   ├── tiers_test.go
│   │   ├── limits.go
│   │   ├── limits_test.go
│   │   ├── tenants.go
│   │   ├── tenants_test.go
│   │   ├── usage.go
│   │   ├── usage_test.go
│   │   ├── transactions.go
│   │   └── transactions_test.go
│   ├── services/                   # Business logic
│   │   ├── tier_service.go
│   │   ├── tier_service_test.go
│   │   ├── limit_service.go
│   │   ├── limit_service_test.go
│   │   ├── tenant_service.go
│   │   ├── tenant_service_test.go
│   │   ├── usage_service.go
│   │   ├── usage_service_test.go
│   │   ├── transaction_service.go
│   │   └── transaction_service_test.go
│   ├── repositories/               # Generic interfaces
│   │   └── interfaces.go
│   ├── models/                     # Domain models
│   │   ├── tier.go
│   │   ├── limit.go
│   │   ├── tenant.go
│   │   ├── usage.go
│   │   └── transaction.go
│   ├── limiter/                    # Limit evaluation engine
│   │   ├── engine.go               # Core evaluation logic
│   │   ├── engine_test.go
│   │   ├── absolute.go
│   │   ├── absolute_test.go
│   │   ├── time_based.go
│   │   ├── time_based_test.go
│   │   ├── cumulative.go
│   │   ├── cumulative_test.go
│   │   ├── burst.go
│   │   ├── burst_test.go
│   │   ├── compound.go
│   │   ├── compound_test.go
│   │   ├── feature.go
│   │   └── feature_test.go
│   ├── errors/                     # Structured error types
│   │   └── errors.go
│   ├── logger/                     # Logger interface + slog adapter
│   │   └── logger.go
│   └── db/                         # Database implementations
│       ├── sqlite/
│       │   ├── repository.go
│       │   ├── repository_test.go  # Integration tests with real SQLite
│       │   └── migrations/
│       ├── postgres/
│       │   ├── repository.go
│       │   └── migrations/
│       └── mongo/
│           └── repository.go
├── pkg/                            # Public SDK/client (future)
├── api/                            # OpenAPI specs (future)
├── PLANNING.md
└── go.mod
```

---

## 3. Data Models

### 3.1 Tier

A tier is a named pricing plan with versioned limit definitions.

```go
type Tier struct {
    ID            string    // ULID
    Key           string    // Unique human-readable key (e.g., "professional")
    Name          string    // Display name (e.g., "Professional Plan")
    Version       int       // Auto-incremented on update
    Deprecated    bool      // If true, no new tenants can subscribe
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

**Key decisions:**
- `Key` is immutable and uniquely identifies a tier across versions
- `Version` is an integer that auto-increments when limits are updated
- Each (Key, Version) pair is a unique tier definition stored as a separate record
- Deprecated tiers cannot accept new tenant subscriptions but existing tenants keep them

### 3.2 Limit Definition

A limit is a constraint attached to a specific tier version. Each limit has a `kind` (type) and kind-specific configuration.

```go
type LimitDefinition struct {
    ID        string            // ULID
    TierID    string            // FK to Tier (specific version)
    Key       string            // Unique within tier (e.g., "maxApiCallsPerMonth")
    Kind      LimitKind         // Enum: absolute, time_based, cumulative, burst, compound, feature
    Config    LimitConfig       // Kind-specific parameters (see below)
    CreatedAt time.Time
}
```

### 3.3 Limit Kinds & Configurations

Each limit kind has its own config structure. In Go code these are typed; in the database, the implementation chooses the best storage (JSONB for Postgres, nested docs for Mongo, structured columns for SQLite).

```go
type LimitKind string

const (
    LimitKindAbsolute   LimitKind = "absolute"    // Fixed cap, never resets automatically
    LimitKindTimeBased  LimitKind = "time_based"   // Resets after a time window
    LimitKindCumulative LimitKind = "cumulative"   // Accumulates over all time (e.g., storage)
    LimitKindBurst      LimitKind = "burst"         // Rate limiting with burst allowance
    LimitKindCompound   LimitKind = "compound"      // Multiple sub-limits ANDed together
    LimitKindFeature    LimitKind = "feature"       // Boolean feature flag (enabled/disabled)
)

// LimitConfig is an interface; each kind has a concrete implementation
type LimitConfig interface {
    Kind() LimitKind
    Validate() error
}

type AbsoluteConfig struct {
    MaxValue int64   // The absolute cap
}

type TimeBasedConfig struct {
    MaxValue int64   // Max per window
    Period   string  // "minute", "hour", "day", "week", "month"
    // Reset is implicit: based on usage record timestamps and window calculation
}

type CumulativeConfig struct {
    MaxValue int64   // Total lifetime cap (e.g., 1GB = 1073741824 bytes)
    Unit     string  // Human-readable unit (e.g., "bytes", "records")
}

type BurstConfig struct {
    MaxRate     int64  // Sustained rate (e.g., 100/sec)
    BurstSize   int64  // Max burst (e.g., 200)
    Window      string // Window for rate averaging (e.g., "minute")
}

type CompoundConfig struct {
    SubLimits []CompoundSubLimit  // Multiple conditions, most restrictive wins
}

type CompoundSubLimit struct {
    Kind   LimitKind   // Any of the above kinds
    Config LimitConfig // Kind-specific config
}

type FeatureConfig struct {
    Enabled bool  // Whether the feature is available in this tier
}
```

### 3.4 Tenant

A tenant is an abstract entity in a hierarchical tree. It can represent a user, department, organization, or any concept the consuming application defines.

```go
type Tenant struct {
    ID          string     // ULID
    ExternalID  string     // The consuming app's identifier for this entity
    ParentID    *string    // Nullable — root tenants have no parent
    TierID      *string    // Nullable — tier-less tenants have no limits
    TierVersion *int       // Which version of the tier this tenant is on
    Status      TenantStatus
    Metadata    map[string]any  // Arbitrary metadata from consuming app
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type TenantStatus string

const (
    TenantStatusActive    TenantStatus = "active"
    TenantStatusSuspended TenantStatus = "suspended"  // Limits not enforced, usage not tracked
    TenantStatusBlocked   TenantStatus = "blocked"     // All operations treated as limit exceeded
)
```

**Key decisions:**
- `ExternalID` is how the consuming application identifies tenants (their user ID, org ID, etc.)
- `ParentID` forms the hierarchy tree — nil means root tenant
- `TierID` + `TierVersion` together pin a tenant to a specific tier version
- Tier-less tenants (`TierID == nil`) have **no limits** — all check/increment/decrement calls succeed
- Two suspension modes (see section 7)

### 3.5 Tenant Limit Scope

Each tenant's relationship to limits has a scope that determines how limits propagate in the hierarchy.

```go
type TenantLimitScope string

const (
    TenantLimitScopeGlobal TenantLimitScope = "global" // Applies to this tenant AND all descendants
    TenantLimitScopeLocal  TenantLimitScope = "local"   // Applies only when explicitly called with this tenant
)
```

**This is defined at the tier level** — when creating a tier's limits, each limit key is marked as global or local. See section 5 for hierarchy rules.

### 3.6 Usage Record

Tracks current consumption of a limit by a tenant. Uses an **aggregated approach** (one record per tenant per limit per active window) rather than storing individual events.

```go
type UsageRecord struct {
    ID          string     // ULID
    TenantID    string     // FK to Tenant
    LimitKey    string     // Which limit this tracks
    CurrentValue int64     // Current aggregated usage
    WindowStart *time.Time // For time-based limits: start of current window
    WindowEnd   *time.Time // For time-based limits: end of current window (implicit reset trigger)
    UpdatedAt   time.Time
}
```

**Key decisions:**
- **Aggregated storage**: one record per (tenant, limit_key, window). No individual event log.
- **Implicit time-based reset**: when checking/incrementing, if `time.Now() > WindowEnd`, the record is considered reset. A new window starts implicitly. No background scheduler needed.
- **Storage footprint**: minimal — only active windows exist in the database.

### 3.7 Tenant Tier History

Audit trail of tier changes for a tenant.

```go
type TenantTierChange struct {
    ID              string     // ULID
    TenantID        string
    PreviousTierID  *string    // nil if first assignment
    PreviousVersion *int
    NewTierID       *string    // nil if unassigning
    NewVersion      *int
    UsageReset      bool       // Whether usage was reset during this change
    ChangedAt       time.Time
}
```

### 3.8 Transaction

Application-level transaction record. **Not a database transaction** — this is a Tierify abstraction stored as a DB record.

```go
type Transaction struct {
    ID          string              // ULID (provided by client via header)
    TenantID    string              // Which tenant this transaction operates on
    LimitKeys   []string            // Which limit keys are locked by this transaction
    Status      TransactionStatus
    TTL         *time.Duration      // Optional, provided on first request
    Operations  []TransactionOp     // Pending operations (increments/decrements)
    CreatedAt   time.Time
    ExpiresAt   *time.Time          // CreatedAt + TTL (nil if no TTL)
}

type TransactionStatus string

const (
    TransactionStatusPending   TransactionStatus = "pending"
    TransactionStatusCommitted TransactionStatus = "committed"
    TransactionStatusRolledBack TransactionStatus = "rolled_back"
    TransactionStatusExpired   TransactionStatus = "expired"
)

type TransactionOp struct {
    LimitKey  string
    Operation string  // "increment" | "decrement"
    Amount    int64
}
```

**Key decisions:**
- Transaction ID comes from the client (via `X-Transaction-ID` header)
- First unknown transaction ID implicitly begins a transaction
- TTL is optional, provided via `X-Transaction-TTL` header on the first request
- Transaction locks prevent other operations on the same (tenant, limit_keys) combination
- On commit: all pending operations are applied atomically
- On rollback: all pending operations are discarded, locks released
- On expiry (TTL exceeded): treated as rollback

---

## 4. Tenant Hierarchy & Federated Limits

### 4.1 Tree Structure

Tenants form a tree via `ParentID`:

```
Organization (root tenant)
├── Department A
│   ├── User 1
│   └── User 2
└── Department B
    └── User 3
```

The tree can be any depth. Root tenants have `ParentID == nil`.

### 4.2 Global vs. Local Limits

Each limit definition in a tier is scoped:

- **Global limits**: Apply to the tenant AND all its descendants. When any descendant increments a global limit, it counts against the ancestor's global limit too.
- **Local limits**: Apply only to the specific tenant they're called on. Descendants don't affect or inherit these.

**Example:**

```
Organization tier limits:
  - "totalApiCalls"  (global, 10000/month)  → shared across all descendants
  - "adminActions"   (local, 500/month)     → only counts org-level admin actions

Department tier limits:
  - "deptApiCalls"   (global, 2000/month)   → shared across dept's users
  - "deptReports"    (local, 100/month)     → only dept-level reports

User tier limits:
  - "userApiCalls"   (local, 500/month)     → per-user limit
```

### 4.3 Increment Logic with Hierarchy

When incrementing a limit for a tenant:

1. Check and increment the tenant's **own** usage (global or local)
2. Walk up the ancestor chain
3. For each ancestor: check and increment only **global** limits with matching keys
4. If **any** limit in the chain is exceeded, the entire operation fails
5. On failure, no usage records are modified (atomic across the chain)

**Pseudocode:**

```
func IncrementLimit(tenantID, limitKey, amount):
    tenant = getTenant(tenantID)
    chain = [tenant] + getAllAncestors(tenant)

    // Phase 1: Check all
    for t in chain:
        if t == tenant:
            check t's limit (global or local) for limitKey
        else:
            if t has a GLOBAL limit for limitKey:
                check t's global limit for limitKey

    // Phase 2: Apply all (only if all checks pass)
    for t in chain:
        if t == tenant:
            increment t's usage for limitKey
        else:
            if t has a GLOBAL limit for limitKey:
                increment t's usage for limitKey
```

### 4.4 Check Logic with Hierarchy

Same traversal as increment but read-only. Returns the **most restrictive** result across the chain.

### 4.5 Tier-less Tenants in Hierarchy

A tier-less tenant (no tier assigned) has **no limits of its own** — all check/increment/decrement on its own limits succeed unconditionally. However:

- **Ancestor global limits still apply.** If a tier-less child increments a limit key that matches an ancestor's global limit, the ancestor's usage is still checked and incremented.
- This means a tier-less user under an organization with "10000 API calls/month" global limit still counts toward the org's quota.
- Only the tenant's own local/global limits are absent; the hierarchy remains enforced.

---

## 5. Tier Versioning, Migration & Deprecation

### 5.1 Version Model

- Each tier has a `Key` (immutable) and a `Version` (integer, auto-incremented)
- Updating a tier's limits creates a **new version** — old versions are preserved
- Each (Key, Version) is a distinct record in the database
- Tenants are pinned to a specific (Key, Version)

### 5.2 Tier Update API

When updating a tier (creating a new version), the caller specifies:

| Parameter | Type | Description |
|---|---|---|
| `migrate_existing` | `bool` | If true, all tenants on any previous version are migrated to the new version |
| `deprecate_previous` | `"none"` \| `"latest"` \| `"all"` | `none`: previous versions stay active. `latest`: only the immediately previous version is deprecated. `all`: all previous versions are deprecated. |
| `reset_usage` | `bool` | If migrating tenants, whether to reset their usage counters |

**Rules:**
- Deprecated versions cannot accept new tenant subscriptions
- Existing tenants on deprecated versions continue to function
- Tenants can be manually registered to previous non-deprecated versions

### 5.3 Manual Tenant Migration

A separate API endpoint allows moving a specific tenant to a different tier/version:

```
PUT /tenants/{tenantId}/tier
{
    "tier_key": "professional",
    "tier_version": 2,        // optional: defaults to latest non-deprecated
    "reset_usage": false       // whether to reset current usage
}
```

---

## 6. Tenant Status & Suspension

### 6.1 Active

Normal operation. Limits are enforced, usage is tracked.

### 6.2 Suspended (Limits Suspension)

- All check/increment/decrement operations **succeed**
- Usage is **not actually incremented** (nothing is tracked)
- Useful for grace periods, testing, or temporary exemptions

### 6.3 Blocked (Tenant Blocking)

- All check/increment/decrement operations **fail** as if limits were reached
- Error responses include `"blocked": true` to distinguish from actual limit exhaustion
- Useful for administrative holds, payment failures, etc.

---

## 7. API Design

All endpoints are prefixed with `/api/v1`.

### 7.1 Tier Management

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/tiers` | Create a new tier |
| `GET` | `/api/v1/tiers` | List all tiers (optionally filter by key, version, deprecated status) |
| `GET` | `/api/v1/tiers/{tierKey}` | Get latest version of a tier |
| `GET` | `/api/v1/tiers/{tierKey}/versions` | List all versions of a tier |
| `GET` | `/api/v1/tiers/{tierKey}/versions/{version}` | Get specific version |
| `PUT` | `/api/v1/tiers/{tierKey}` | Update tier (creates new version) |
| `DELETE` | `/api/v1/tiers/{tierKey}/versions/{version}` | Deprecate a specific version |

### 7.2 Limit Definitions

Limits are managed as part of a tier's definition (nested in tier create/update), but also queryable independently.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/tiers/{tierKey}/versions/{version}/limits` | List limits for a tier version |
| `GET` | `/api/v1/tiers/{tierKey}/versions/{version}/limits/{limitKey}` | Get specific limit definition |

### 7.3 Tenant Management

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/tenants` | Create tenant (optionally with tier, parent) |
| `GET` | `/api/v1/tenants/{tenantId}` | Get tenant details |
| `GET` | `/api/v1/tenants/{tenantId}/children` | List direct children (paginated) |
| `GET` | `/api/v1/tenants/{tenantId}/ancestors` | List ancestor chain |
| `PUT` | `/api/v1/tenants/{tenantId}/tier` | Assign/change tier (with reset_usage option) |
| `PUT` | `/api/v1/tenants/{tenantId}/status` | Change status (active/suspended/blocked) |
| `GET` | `/api/v1/tenants/{tenantId}/history` | Get tier change history |
| `DELETE` | `/api/v1/tenants/{tenantId}` | Remove tenant (cascade or orphan mode via query param) |

### 7.4 Usage Operations

These are the core operational APIs. All support optional transaction headers.

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/tenants/{tenantId}/check` | Check one or more limits (read-only) |
| `POST` | `/api/v1/tenants/{tenantId}/increment` | Increment one or more limits |
| `POST` | `/api/v1/tenants/{tenantId}/decrement` | Decrement one or more limits |
| `POST` | `/api/v1/tenants/{tenantId}/reset` | Reset specific limits (manual reset) |
| `GET` | `/api/v1/tenants/{tenantId}/usage` | Get current usage for all limits |
| `GET` | `/api/v1/tenants/{tenantId}/usage/{limitKey}` | Get current usage for specific limit |

**Request body for check/increment/decrement:**

```json
{
    "operations": [
        { "limit_key": "apiCalls", "amount": 1 },
        { "limit_key": "storageBytes", "amount": 1048576 }
    ]
}
```

**Transaction headers (optional):**

| Header | Description |
|---|---|
| `X-Transaction-ID` | Client-provided transaction ID. First unknown ID implicitly begins a transaction. |
| `X-Transaction-TTL` | TTL in seconds for the transaction (only on first request with this ID) |

### 7.5 Tenant Deletion

The `DELETE /api/v1/tenants/{tenantId}` endpoint accepts a query parameter to control child handling:

| Query Param | Value | Behavior |
|---|---|---|
| `mode` | `cascade` | Delete the tenant and all descendants recursively |
| `mode` | `orphan` | Delete the tenant; children become parentless (ParentID set to nil) |

If `mode` is omitted, the API returns **400 Bad Request** (must be explicit).

### 7.6 Transaction Management

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/transactions/{txId}/commit` | Commit all pending operations |
| `POST` | `/api/v1/transactions/{txId}/rollback` | Rollback all pending operations |
| `GET` | `/api/v1/transactions/{txId}` | Get transaction status and pending operations |

### 7.7 Pagination

All list endpoints use **offset-based pagination** with query parameters:

| Query Param | Default | Description |
|---|---|---|
| `offset` | `0` | Number of records to skip |
| `limit` | `20` | Number of records to return (max 100) |

Response includes pagination metadata:

```json
{
    "data": [...],
    "pagination": {
        "offset": 0,
        "limit": 20,
        "total": 150
    }
}
```

### 7.8 Multi-Operation Atomicity

When a check/increment/decrement request contains multiple operations (e.g., check `apiCalls` AND `storage`):

- **All-or-nothing**: if any single limit check fails, the entire request fails
- The error response **always** contains an array of results for **all** operations, each indicating its individual status
- This applies even when only one limit is in the request (always an array)

```json
{
    "error": "limit_exceeded",
    "tenant_id": "01HQXYZ...",
    "blocked": false,
    "details": [
        {
            "limit_key": "apiCalls",
            "limit_kind": "time_based",
            "exceeded": true,
            "current_value": 100,
            "limit_value": 100,
            "remaining": 0,
            "reset_at": "2025-02-01T00:00:00Z",
            "retry_after_seconds": 3600
        },
        {
            "limit_key": "storage",
            "limit_kind": "cumulative",
            "exceeded": false,
            "current_value": 500,
            "limit_value": 1000,
            "remaining": 500
        }
    ]
}
```

### 7.9 Success Response Format (Check)

```json
{
    "results": [
        {
            "limit_key": "apiCalls",
            "available": true,
            "current_value": 50,
            "limit_value": 100,
            "remaining": 50,
            "limit_kind": "time_based",
            "reset_at": "2025-02-01T00:00:00Z"
        }
    ]
}
```

### 7.10 Error Response Format (Limit Exceeded)

```json
{
    "error": "limit_exceeded",
    "tenant_id": "uuid-here",
    "blocked": false,
    "details": [
        {
            "limit_key": "apiCalls",
            "limit_kind": "time_based",
            "current_value": 100,
            "limit_value": 100,
            "remaining": 0,
            "reset_at": "2025-02-01T00:00:00Z",
            "retry_after_seconds": 3600
        }
    ]
}
```

For blocked tenants, `blocked: true` is set and `details` may be empty.

For suspended tenants, operations succeed with a `"suspended": true` flag in the response.

---

## 8. Transaction System

### 8.1 Overview

Tierify transactions are an **application-level abstraction**, not database transactions. They are stored as records in the database and enforced by the service layer.

### 8.2 Lifecycle

```
Client sends request with X-Transaction-ID: "tx-123" (unknown ID)
    → Tierify creates Transaction record (status: pending)
    → Locks (tenant_id, limit_keys) for this transaction
    → Records the operation (e.g., increment apiCalls by 1)
    → Returns success (but usage is NOT yet applied)

Client sends more requests with X-Transaction-ID: "tx-123"
    → Additional operations recorded
    → Locks extended if new limit_keys are involved

Client sends POST /transactions/tx-123/commit
    → All pending operations applied atomically
    → Locks released
    → Transaction status → committed

OR Client sends POST /transactions/tx-123/rollback
    → All pending operations discarded
    → Locks released
    → Transaction status → rolled_back

OR TTL expires
    → Same as rollback
    → Transaction status → expired
```

### 8.3 Locking Semantics

- A transaction locks **(tenant_id, limit_key)** tuples
- While locked, **no other operation** (transactional or non-transactional) can modify those limits for that tenant
- Non-transactional operations on locked limits receive a **409 Conflict** with transaction info
- Two transactions trying to lock the same tuple: second one receives **409 Conflict**

### 8.4 Optimistic (Non-Transactional) Mode

Without transaction headers:
- Check is read-only, no side effects
- Increment/decrement are immediately applied
- No locking, no reservations
- Race conditions possible (check may pass but increment may fail if someone else incremented between)

### 8.5 Future: Reservation System

For optimistic mode, a future enhancement could add a reservation system with TTL:
- Check with `X-Reserve: true` header temporarily reserves capacity
- Reservation has a TTL
- Increment within TTL succeeds if reservation is valid
- This is **not in MVP scope** — just noted for future design

---

## 9. Repository Interfaces

### 9.1 Generic Interface Design

All database implementations must satisfy the same Go interfaces. The interfaces use standard CRUD patterns plus domain-specific operations.

```go
type TierRepository interface {
    Create(ctx context.Context, tier *models.Tier, limits []models.LimitDefinition) error
    GetByKey(ctx context.Context, key string) (*models.Tier, error)                           // Latest non-deprecated version
    GetByKeyAndVersion(ctx context.Context, key string, version int) (*models.Tier, error)
    ListVersions(ctx context.Context, key string) ([]models.Tier, error)
    List(ctx context.Context, filters TierFilters) ([]models.Tier, error)
    Update(ctx context.Context, key string, limits []models.LimitDefinition, opts TierUpdateOptions) (*models.Tier, error)
    Deprecate(ctx context.Context, key string, version int) error
}

type TenantRepository interface {
    Create(ctx context.Context, tenant *models.Tenant) error
    GetByID(ctx context.Context, id string) (*models.Tenant, error)
    GetByExternalID(ctx context.Context, externalID string) (*models.Tenant, error)
    GetChildren(ctx context.Context, tenantID string) ([]models.Tenant, error)
    GetAncestors(ctx context.Context, tenantID string) ([]models.Tenant, error)
    UpdateTier(ctx context.Context, tenantID string, tierKey string, version int, resetUsage bool) error
    UpdateStatus(ctx context.Context, tenantID string, status models.TenantStatus) error
    Delete(ctx context.Context, tenantID string) error
    RecordTierChange(ctx context.Context, change *models.TenantTierChange) error
    GetTierHistory(ctx context.Context, tenantID string) ([]models.TenantTierChange, error)
}

type UsageRepository interface {
    GetUsage(ctx context.Context, tenantID string, limitKey string) (*models.UsageRecord, error)
    GetAllUsage(ctx context.Context, tenantID string) ([]models.UsageRecord, error)
    IncrementUsage(ctx context.Context, tenantID string, limitKey string, amount int64) (*models.UsageRecord, error)
    DecrementUsage(ctx context.Context, tenantID string, limitKey string, amount int64) (*models.UsageRecord, error)
    ResetUsage(ctx context.Context, tenantID string, limitKey string) error
    ResetAllUsage(ctx context.Context, tenantID string) error
}

type LimitDefinitionRepository interface {
    GetByTier(ctx context.Context, tierID string) ([]models.LimitDefinition, error)
    GetByTierAndKey(ctx context.Context, tierID string, limitKey string) (*models.LimitDefinition, error)
}

type TransactionRepository interface {
    Create(ctx context.Context, tx *models.Transaction) error
    GetByID(ctx context.Context, id string) (*models.Transaction, error)
    AddOperation(ctx context.Context, txID string, op models.TransactionOp) error
    UpdateStatus(ctx context.Context, txID string, status models.TransactionStatus) error
    GetPendingByTenantAndLimitKey(ctx context.Context, tenantID string, limitKey string) (*models.Transaction, error)
    CleanExpired(ctx context.Context) (int, error)  // Expire transactions past TTL
}
```

### 9.2 Database-Specific Implementations

Each database uses the schema layout that best fits its strengths:

| Aspect | SQLite | PostgreSQL | MongoDB |
|---|---|---|---|
| Schema | Strict relational tables | Relational + JSONB for limit configs | Document-based, nested limit configs |
| Limit config storage | Separate columns per config field, or JSON text column | JSONB column | Embedded subdocument |
| Hierarchy queries | Recursive CTEs | Recursive CTEs | `$graphLookup` aggregation |
| Atomic increment | `UPDATE ... SET value = value + ?` | Same + `RETURNING` | `$inc` operator |
| Locking (transactions) | Record-based check | Advisory locks or record-based | Document-level locks |

### 9.3 Repository Registration

A factory pattern selects the right implementation at startup:

```go
func NewRepositories(dbType string, connectionString string) (*Repositories, error) {
    switch dbType {
    case "sqlite":
        return sqlite.NewRepositories(connectionString)
    case "postgres":
        return postgres.NewRepositories(connectionString)
    case "mongo":
        return mongo.NewRepositories(connectionString)
    default:
        return nil, fmt.Errorf("unsupported database type: %s", dbType)
    }
}
```

---

## 10. Limit Evaluation Engine

The `limiter` package encapsulates the logic for evaluating whether an operation is allowed based on limit kind.

### 10.1 Engine Interface

```go
type Engine interface {
    Check(ctx context.Context, definition *models.LimitDefinition, usage *models.UsageRecord, amount int64) (*CheckResult, error)
    // Returns whether the operation is allowed and detailed status
}

type CheckResult struct {
    Allowed        bool
    CurrentValue   int64
    LimitValue     int64
    Remaining      int64
    LimitKind      models.LimitKind
    ResetAt        *time.Time  // For time-based limits
    RetryAfterSecs *int64      // Seconds until retry makes sense
}
```

### 10.2 Per-Kind Evaluation

Each limit kind has its own evaluator:

- **Absolute**: `currentValue + amount <= maxValue`
- **Time-based**: Check if current window is active (based on `WindowStart`/`WindowEnd` and `time.Now()`). If window expired, implicit reset (current = 0). Then `currentValue + amount <= maxValue`.
- **Cumulative**: Same as absolute but with lifetime semantics (no reset ever)
- **Burst**: Token bucket or sliding window algorithm. Check average rate over window and instantaneous burst.
- **Compound**: Evaluate all sub-limits. All must pass (AND logic). Return the most restrictive result.
- **Feature**: `enabled == true` → allowed; `enabled == false` → denied.

---

## 11. Time-Based Reset Strategy

### 11.1 Implicit Reset via Timestamps

No background scheduler. Instead:

1. Each usage record stores `WindowStart` and `WindowEnd`
2. On every check/increment/decrement, the service checks: `time.Now() > WindowEnd`?
3. If yes: the window has expired → treat `CurrentValue` as `0`, set new `WindowStart = now`, compute new `WindowEnd` based on period
4. If no: use existing `CurrentValue`

**Benefits:**
- Completely stateless service (no cron jobs, no schedulers)
- Works naturally with horizontal scaling
- Reset precision is exact (checked on every operation)

**Trade-off:**
- If a tenant is inactive for a long period, their old records remain in the DB until next access. A periodic cleanup job can handle this (optional, not MVP).

### 11.2 Manual Reset

The `POST /tenants/{tenantId}/reset` endpoint allows explicit reset of specific limits:

```json
{
    "limit_keys": ["apiCalls", "storageBytes"]
}
```

Omitting `limit_keys` resets all limits for the tenant.

---

## 12. Typical Usage Flow

### 12.1 Optimistic (No Transaction)

```
1. Client: POST /tenants/{id}/check
   Body: { operations: [{ limit_key: "apiCalls", amount: 1 }] }
   Response: { results: [{ available: true, remaining: 49 }] }

2. Client performs its business logic (e.g., processes an API call)

3. Client: POST /tenants/{id}/increment
   Body: { operations: [{ limit_key: "apiCalls", amount: 1 }] }
   Response: { results: [{ current_value: 51, remaining: 49 }] }

   (If between step 1 and 3 another request pushed usage to 100,
    this returns 429 with limit_exceeded error)
```

### 12.2 Transactional

```
1. Client: POST /tenants/{id}/check
   Headers: X-Transaction-ID: tx-abc, X-Transaction-TTL: 30
   Body: { operations: [{ limit_key: "apiCalls", amount: 1 }, { limit_key: "storage", amount: 1024 }] }
   Response: { results: [...], transaction: { id: "tx-abc", status: "pending" } }
   → Locks (tenant, apiCalls) and (tenant, storage)

2. Client performs its business logic

3. Client: POST /tenants/{id}/increment
   Headers: X-Transaction-ID: tx-abc
   Body: { operations: [{ limit_key: "apiCalls", amount: 1 }] }
   Response: { results: [...], transaction: { id: "tx-abc", operations: 2 } }

4. Client: POST /transactions/tx-abc/commit
   Response: { status: "committed", operations_applied: 2 }
   → Usage updated, locks released

   OR

4. Client: POST /transactions/tx-abc/rollback
   Response: { status: "rolled_back" }
   → Nothing applied, locks released
```

---

## 13. Implementation Roadmap

Every phase includes writing tests **alongside** (or before) the implementation code.

### Phase 1 — Foundation (MVP)

- [ ] Project scaffolding (Go modules, directory structure, config, logger)
- [ ] Domain models (Tier, Limit, Tenant, Usage, Transaction)
- [ ] Repository interfaces
- [ ] SQLite repository implementation + integration tests
- [ ] Limit evaluation engine (absolute, time-based, feature limits) + unit tests
- [ ] Core services (tier, tenant, usage) + unit tests with mocked repos
- [ ] HTTP handlers with chi routing + handler tests
- [ ] Structured error responses
- [ ] Check, Increment, Decrement, Reset APIs
- [ ] Tier CRUD with versioning and deprecation
- [ ] Tenant CRUD with tier assignment (including cascade/orphan delete)
- [ ] Tier-less tenant behavior
- [ ] Manual reset
- [ ] Decrement below zero behavior (controlled by `ALLOW_USAGE_BELOW_ZERO`)
- [ ] Offset-based pagination for list endpoints

### Phase 2 — Hierarchy & Advanced Limits

- [ ] Tenant hierarchy (parent-child relationships)
- [ ] Global vs. local limit scoping
- [ ] Federated limit checking (ancestor chain traversal) + tests for deep hierarchies
- [ ] Tier-less tenants within hierarchy (ancestor global limits still apply)
- [ ] Cumulative, burst, compound limit kinds + unit tests
- [ ] Tier migration API (version management, deprecation controls)
- [ ] Tenant tier change history

### Phase 3 — Transactions

- [ ] Transaction model and repository
- [ ] Transaction creation (implicit begin via header)
- [ ] Transaction locking mechanism
- [ ] Commit and rollback APIs
- [ ] TTL expiration handling
- [ ] Conflict detection (409 responses)
- [ ] Transaction integration tests (concurrency, TTL expiry, lock conflicts)

### Phase 4 — Suspension & Blocking

- [ ] Tenant status management (active/suspended/blocked)
- [ ] Suspended mode (operations succeed, usage not tracked)
- [ ] Blocked mode (operations fail with `blocked: true` flag)

### Phase 5 — Additional Database Backends

- [ ] PostgreSQL repository implementation + integration tests
- [ ] MongoDB repository implementation + integration tests
- [ ] Database factory and configuration

### Phase 6 — Polish & Future

- [ ] Reservation system for optimistic mode (future)
- [ ] API documentation (OpenAPI spec)
- [ ] Performance testing and optimization
- [ ] Horizontal scaling validation

---

## 14. Configuration

All configuration is via environment variables, validated at startup using `github.com/caarlos0/env/v11`. Collected into a single `Config` struct injected throughout the application.

```go
type Config struct {
    // Server
    Port            int           `env:"PORT" envDefault:"8080"`
    Host            string        `env:"HOST" envDefault:"0.0.0.0"`
    ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"15s"`
    WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"15s"`

    // Database
    DBType          string        `env:"DB_TYPE" envDefault:"sqlite"`       // sqlite, postgres, mongo
    DBConnectionStr string        `env:"DB_CONNECTION_STRING" envDefault:"tierify.db"`

    // Behavior
    AllowUsageBelowZero bool      `env:"ALLOW_USAGE_BELOW_ZERO" envDefault:"false"`

    // Logging
    LogLevel        string        `env:"LOG_LEVEL" envDefault:"info"`       // debug, info, warn, error
    LogFormat       string        `env:"LOG_FORMAT" envDefault:"json"`      // json, text
}
```

**`ALLOW_USAGE_BELOW_ZERO`**: When `false` (default), decrement operations that would bring usage below 0 are rejected with an error. When `true`, usage can go negative (useful for credits/refunds).

---

## 15. Logging

A `Logger` interface wraps the actual logging implementation, allowing the underlying library to be swapped without changing application code.

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger
}
```

Default implementation uses `log/slog` from stdlib. Can be swapped to `zerolog`, `zap`, etc. by implementing the interface.

---

## 16. Testing Strategy

### 16.1 Philosophy

Near-TDD approach: write tests alongside (or before) implementation. Every public function and every behavior must be tested.

### 16.2 Test Levels

| Level | Scope | Dependencies | Location |
|---|---|---|---|
| **Unit tests** | Individual functions, service methods, limiter logic | Mocked repositories (interfaces) | `*_test.go` next to source |
| **Integration tests** | Repository implementations against real DB | Real SQLite (in-memory) | `db/sqlite/*_test.go` |
| **Handler tests** | HTTP request/response cycle | Mocked services | `handlers/*_test.go` |

### 16.3 Mocking Strategy

- Repository interfaces are mocked for service tests (using `testify/mock` or hand-written mocks)
- Service interfaces are mocked for handler tests
- SQLite integration tests use `:memory:` database with migrations applied per test

### 16.4 Test Coverage Targets

- **Limiter engine**: 100% — every limit kind, every edge case (boundary, overflow, window expiry)
- **Services**: 100% of business logic paths — happy path, error cases, hierarchy traversal, transaction states
- **Handlers**: Request parsing, response formatting, status codes, error shapes
- **Repositories**: CRUD operations, constraint violations, concurrent access

---

## 17. Open Discussion Points

Items to revisit in future planning sessions:

1. **Reservation system**: How should optimistic reservations work? TTL, conflict resolution, partial reservations.
2. **Cleanup jobs**: Periodic cleanup of expired usage records, expired transactions, audit logs.
3. **Observability**: Metrics and tracing strategy (logging is decided: slog behind interface).
4. **Bulk operations**: Batch check/increment for multiple tenants at once.
5. **Webhooks/events**: Should Tierify emit events when limits are hit, tiers changed, etc.?
6. **Client SDK**: Go client library for consuming applications.

---

## 18. Design Decisions Log

| # | Decision | Rationale |
|---|---|---|
| 1 | Flexible limit kinds via interface + config pattern | Extensibility — new limit kinds can be added without schema changes |
| 2 | Aggregated usage storage (not event-sourced) | Storage efficiency — one record per tenant per limit per window |
| 3 | Implicit time-based reset via timestamps | Stateless service — no schedulers, horizontal scaling friendly |
| 4 | Application-level transactions (not DB transactions) | DB-agnostic — works across SQLite, Postgres, Mongo |
| 5 | Tenant hierarchy with global/local limit scopes | Maximum flexibility for consuming applications |
| 6 | Tier versioning with explicit migration control | Safe rollouts — existing tenants aren't affected unless explicitly migrated |
| 7 | Per-DB schema layout (not isomorphic) | Each DB uses its strengths (JSONB, documents, strict columns) |
| 8 | Completely stateless service | Horizontal scaling — any instance can handle any request |
| 9 | Two suspension modes (suspended vs blocked) | Different business needs — grace periods vs administrative holds |
| 10 | Tier-less tenants have no limits (always succeed) | Simple default — consuming app assigns tier when ready |
| 11 | `chi` router | Lightweight, stdlib-compatible, good middleware ecosystem |
| 12 | ULIDs for all IDs | Sortable, DB-index friendly, no coordination needed |
| 13 | Offset-based pagination | Simpler than cursor-based; sufficient for admin/backend use |
| 14 | Multi-operation atomicity (all-or-nothing) | Prevents partial state; errors always return full array of results |
| 15 | `ALLOW_USAGE_BELOW_ZERO` env var (default false) | Configurable per deployment; supports both strict and credit-based usage |
| 16 | Tenant deletion requires explicit mode (cascade/orphan) | Prevents accidental data loss; forces client to be explicit |
| 17 | Ancestor's global limits apply to tier-less children | Hierarchy enforcement is consistent; tier-less only means "no own limits" |
| 18 | `caarlos0/env` for config with injected struct | Single source of truth for config; validated at startup; consistent injection |
| 19 | Logger interface over slog | Swappable logging backend without changing application code |
| 20 | Near-TDD with unit + integration tests | High confidence; catches regressions; tests document behavior |
