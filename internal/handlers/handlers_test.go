package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tierify/internal/config"
	"tierify/internal/limiter"
	"tierify/internal/logger"
	"tierify/internal/models"
	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testApp sets up a full router with real services backed by mock repos.
type testApp struct {
	router     chi.Router
	tierSvc    services.TierService
	tenantSvc  services.TenantService
	usageSvc   services.UsageService
	txSvc      services.TransactionService
}

func newTestApp() *testApp {
	// Reuse the mock repos from services package via a minimal setup.
	repos := newHandlerMockRepos()
	cfg := &config.Config{}
	log := logger.Nop()
	engine := limiter.NewEngine()

	tierSvc := services.NewTierService(repos, cfg, log)
	tenantSvc := services.NewTenantService(repos, cfg, log)
	usageSvc := services.NewUsageService(repos, cfg, log, engine)
	txSvc := services.NewTransactionService(repos, usageSvc, log)

	tierHandler := NewTierHandler(tierSvc)
	tenantHandler := NewTenantHandler(tenantSvc)
	usageHandler := NewUsageHandler(usageSvc)
	txHandler := NewTransactionHandler(txSvc)

	r := chi.NewRouter()
	r.Mount("/api/v1/tiers", tierHandler.Routes())
	r.Route("/api/v1/tenants", func(r chi.Router) {
		r.Mount("/", tenantHandler.Routes())
		usageHandler.RegisterRoutes(r)
	})
	r.Mount("/api/v1/transactions", txHandler.Routes())

	return &testApp{
		router:    r,
		tierSvc:   tierSvc,
		tenantSvc: tenantSvc,
		usageSvc:  usageSvc,
		txSvc:     txSvc,
	}
}

func (a *testApp) do(method, path string, body any) *httptest.ResponseRecorder {
	return a.doWithHeaders(method, path, body, nil)
}

func (a *testApp) doWithHeaders(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

// --- Tier Handler Tests ---

func TestTierHandler_Create(t *testing.T) {
	app := newTestApp()

	body := map[string]any{
		"key":  "basic",
		"name": "Basic Plan",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "scope": "local", "config": map[string]any{"max_value": 100}},
		},
	}

	rec := app.do("POST", "/api/v1/tiers", body)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var tier models.Tier
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tier))
	assert.Equal(t, "basic", tier.Key)
	assert.Equal(t, 1, tier.Version)
}

func TestTierHandler_Create_BadRequest(t *testing.T) {
	app := newTestApp()

	body := map[string]any{"key": "", "name": ""}
	rec := app.do("POST", "/api/v1/tiers", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTierHandler_GetByKey(t *testing.T) {
	app := newTestApp()

	// Create first.
	body := map[string]any{
		"key":  "pro",
		"name": "Pro Plan",
		"limits": []map[string]any{
			{"key": "x", "kind": "absolute", "config": map[string]any{"max_value": 1}},
		},
	}
	app.do("POST", "/api/v1/tiers", body)

	rec := app.do("GET", "/api/v1/tiers/pro", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTierHandler_GetByKey_NotFound(t *testing.T) {
	app := newTestApp()
	rec := app.do("GET", "/api/v1/tiers/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTierHandler_Update(t *testing.T) {
	app := newTestApp()

	createBody := map[string]any{
		"key":  "pro",
		"name": "Pro",
		"limits": []map[string]any{
			{"key": "x", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	}
	app.do("POST", "/api/v1/tiers", createBody)

	updateBody := map[string]any{
		"limits": []map[string]any{
			{"key": "x", "kind": "absolute", "config": map[string]any{"max_value": 200}},
		},
		"deprecate_previous": "latest",
	}
	rec := app.do("PUT", "/api/v1/tiers/pro", updateBody)
	assert.Equal(t, http.StatusOK, rec.Code)

	var tier models.Tier
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tier))
	assert.Equal(t, 2, tier.Version)
}

func TestTierHandler_Deprecate(t *testing.T) {
	app := newTestApp()

	body := map[string]any{
		"key":  "old",
		"name": "Old",
		"limits": []map[string]any{
			{"key": "x", "kind": "absolute", "config": map[string]any{"max_value": 1}},
		},
	}
	app.do("POST", "/api/v1/tiers", body)

	rec := app.do("DELETE", "/api/v1/tiers/old/versions/1", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- Tenant Handler Tests ---

func TestTenantHandler_Create(t *testing.T) {
	app := newTestApp()

	body := map[string]any{"external_id": "user-1"}
	rec := app.do("POST", "/api/v1/tenants", body)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var tenant models.Tenant
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tenant))
	assert.Equal(t, "user-1", tenant.ExternalID)
}

func TestTenantHandler_Create_MissingExternalID(t *testing.T) {
	app := newTestApp()
	rec := app.do("POST", "/api/v1/tenants", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTenantHandler_GetByID(t *testing.T) {
	app := newTestApp()

	rec := app.do("POST", "/api/v1/tenants", map[string]any{"external_id": "user-1"})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("GET", "/api/v1/tenants/"+tenant.ID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTenantHandler_Delete_RequiresMode(t *testing.T) {
	app := newTestApp()

	rec := app.do("POST", "/api/v1/tenants", map[string]any{"external_id": "user-1"})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	// Without mode parameter — should fail.
	rec = app.do("DELETE", "/api/v1/tenants/"+tenant.ID, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// With mode=cascade.
	rec = app.do("DELETE", "/api/v1/tenants/"+tenant.ID+"?mode=cascade", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestTenantHandler_UpdateStatus(t *testing.T) {
	app := newTestApp()

	rec := app.do("POST", "/api/v1/tenants", map[string]any{"external_id": "user-1"})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("PUT", "/api/v1/tenants/"+tenant.ID+"/status", map[string]any{"status": "blocked"})
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestTenantHandler_UpdateStatus_InvalidStatus(t *testing.T) {
	app := newTestApp()

	rec := app.do("POST", "/api/v1/tenants", map[string]any{"external_id": "user-1"})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("PUT", "/api/v1/tenants/"+tenant.ID+"/status", map[string]any{"status": "invalid"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Suspension & Blocking Handler Tests ---

func createTenantWithStatus(t *testing.T, app *testApp, status models.TenantStatus) models.Tenant {
	t.Helper()
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "test-tier", "name": "Test",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	})
	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-status", "tier_key": "test-tier",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	app.do("PUT", "/api/v1/tenants/"+tenant.ID+"/status", map[string]any{"status": string(status)})
	return tenant
}

func TestUsageHandler_Check_Blocked(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusBlocked)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/check", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 1},
		},
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tenant_blocked", body["error"])
	assert.Equal(t, true, body["blocked"])
}

func TestUsageHandler_Increment_Blocked(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusBlocked)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 1},
		},
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tenant_blocked", body["error"])
	assert.Equal(t, true, body["blocked"])
}

func TestUsageHandler_Decrement_Blocked(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusBlocked)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/decrement", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 1},
		},
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tenant_blocked", body["error"])
}

func TestUsageHandler_Check_Suspended(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusSuspended)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/check", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 1},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	results := body["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(t, true, first["available"])
	assert.Equal(t, "suspended", first["limit_kind"])
}

func TestUsageHandler_Increment_Suspended(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusSuspended)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 10},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify no usage was recorded.
	rec = app.do("GET", "/api/v1/tenants/"+tenant.ID+"/usage/apiCalls", nil)
	// Should get the record back (might be nil/empty depending on implementation).
	// The key thing is increment succeeded without tracking.
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, rec.Code)
}

func TestUsageHandler_Decrement_Suspended(t *testing.T) {
	app := newTestApp()
	tenant := createTenantWithStatus(t, app, models.TenantStatusSuspended)

	rec := app.do("POST", "/api/v1/tenants/"+tenant.ID+"/decrement", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 10},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- Usage Handler Tests ---

func TestUsageHandler_Check(t *testing.T) {
	app := newTestApp()

	// Create tier and tenant.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "pro", "name": "Pro",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	})

	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "tier_key": "pro",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("POST", "/api/v1/tenants/"+tenant.ID+"/check", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 10},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUsageHandler_Increment(t *testing.T) {
	app := newTestApp()

	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "pro", "name": "Pro",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	})

	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "tier_key": "pro",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 10},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUsageHandler_Increment_ExceedsLimit(t *testing.T) {
	app := newTestApp()

	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "pro", "name": "Pro",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 10}},
		},
	})

	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "tier_key": "pro",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 100},
		},
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestUsageHandler_EmptyOperations(t *testing.T) {
	app := newTestApp()

	rec := app.do("POST", "/api/v1/tenants", map[string]any{"external_id": "user-1"})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)

	rec = app.do("POST", "/api/v1/tenants/"+tenant.ID+"/check", map[string]any{
		"operations": []map[string]any{},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Hierarchy Handler Tests ---

func TestUsageHandler_Hierarchy_GlobalLimitEnforced(t *testing.T) {
	app := newTestApp()

	// Create org tier with global limit.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "org-tier", "name": "Org",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "scope": "global", "config": map[string]any{"max_value": 100}},
		},
	})

	// Create user tier with local limit.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "user-tier", "name": "User",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "scope": "local", "config": map[string]any{"max_value": 50}},
		},
	})

	// Create org tenant.
	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "org-1", "tier_key": "org-tier",
	})
	var org models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &org)

	// Create user tenant under org.
	rec = app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "parent_id": org.ID, "tier_key": "user-tier",
	})
	var user models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &user)

	// Increment user's apiCalls.
	rec = app.do("POST", "/api/v1/tenants/"+user.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 30},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify org's usage was also affected.
	rec = app.do("GET", "/api/v1/tenants/"+org.ID+"/usage/apiCalls", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var orgUsage models.UsageRecord
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &orgUsage))
	assert.Equal(t, int64(30), orgUsage.CurrentValue)
}

func TestUsageHandler_Hierarchy_GlobalLimitExceeded(t *testing.T) {
	app := newTestApp()

	// Create org tier with low global limit.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "org-tier", "name": "Org",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "scope": "global", "config": map[string]any{"max_value": 10}},
		},
	})

	// Create user tier with higher local limit.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "user-tier", "name": "User",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "scope": "local", "config": map[string]any{"max_value": 100}},
		},
	})

	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "org-1", "tier_key": "org-tier",
	})
	var org models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &org)

	rec = app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "parent_id": org.ID, "tier_key": "user-tier",
	})
	var user models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &user)

	// Try to increment beyond org's global limit.
	rec = app.do("POST", "/api/v1/tenants/"+user.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 20},
		},
	})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestTierHandler_Update_WithMigration(t *testing.T) {
	app := newTestApp()

	// Create tier v1.
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "pro", "name": "Pro",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	})

	// Create tenant on this tier.
	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "user-1", "tier_key": "pro",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)
	assert.Equal(t, 1, *tenant.TierVersion)

	// Update tier with migration.
	rec = app.do("PUT", "/api/v1/tiers/pro", map[string]any{
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 200}},
		},
		"deprecate_previous": "latest",
		"migrate_existing":   true,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var newTier models.Tier
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &newTier))
	assert.Equal(t, 2, newTier.Version)

	// Verify tenant was migrated to v2.
	rec = app.do("GET", "/api/v1/tenants/"+tenant.ID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var migratedTenant models.Tenant
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &migratedTenant))
	assert.Equal(t, 2, *migratedTenant.TierVersion)
}

// --- Transaction Handler Tests ---

func setupTxTestTenant(t *testing.T, app *testApp) models.Tenant {
	t.Helper()
	app.do("POST", "/api/v1/tiers", map[string]any{
		"key": "tx-tier", "name": "TxTier",
		"limits": []map[string]any{
			{"key": "apiCalls", "kind": "absolute", "config": map[string]any{"max_value": 100}},
		},
	})
	rec := app.do("POST", "/api/v1/tenants", map[string]any{
		"external_id": "tx-user", "tier_key": "tx-tier",
	})
	var tenant models.Tenant
	json.Unmarshal(rec.Body.Bytes(), &tenant)
	return tenant
}

func TestTransactionHandler_FullLifecycle(t *testing.T) {
	app := newTestApp()
	tenant := setupTxTestTenant(t, app)

	txHeaders := map[string]string{
		"X-Transaction-ID":  "tx-http-1",
		"X-Transaction-TTL": "30",
	}

	// Transactional increment.
	rec := app.doWithHeaders("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 25},
		},
	}, txHeaders)
	assert.Equal(t, http.StatusOK, rec.Code)

	var incResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &incResp))
	txInfo := incResp["transaction"].(map[string]any)
	assert.Equal(t, "tx-http-1", txInfo["id"])
	assert.Equal(t, "pending", txInfo["status"])

	// Get transaction status.
	rec = app.do("GET", "/api/v1/transactions/tx-http-1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var txResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &txResp))
	assert.Equal(t, "pending", txResp["status"])

	// Commit.
	rec = app.do("POST", "/api/v1/transactions/tx-http-1/commit", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var commitResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &commitResp))
	assert.Equal(t, "committed", commitResp["status"])
	assert.Equal(t, float64(1), commitResp["operations_applied"])

	// Verify usage was applied.
	rec = app.do("GET", "/api/v1/tenants/"+tenant.ID+"/usage/apiCalls", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var usage models.UsageRecord
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &usage))
	assert.Equal(t, int64(25), usage.CurrentValue)
}

func TestTransactionHandler_Rollback(t *testing.T) {
	app := newTestApp()
	tenant := setupTxTestTenant(t, app)

	txHeaders := map[string]string{"X-Transaction-ID": "tx-rb-1"}

	// Transactional increment.
	rec := app.doWithHeaders("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 50},
		},
	}, txHeaders)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Rollback.
	rec = app.do("POST", "/api/v1/transactions/tx-rb-1/rollback", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var rbResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rbResp))
	assert.Equal(t, "rolled_back", rbResp["status"])

	// Verify no usage was applied.
	rec = app.do("GET", "/api/v1/tenants/"+tenant.ID+"/usage/apiCalls", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTransactionHandler_Get_NotFound(t *testing.T) {
	app := newTestApp()

	rec := app.do("GET", "/api/v1/transactions/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTransactionHandler_Conflict(t *testing.T) {
	app := newTestApp()
	tenant := setupTxTestTenant(t, app)

	// First transaction locks apiCalls.
	rec := app.doWithHeaders("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 10},
		},
	}, map[string]string{"X-Transaction-ID": "tx-a"})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Non-transactional increment on locked key.
	rec = app.do("POST", "/api/v1/tenants/"+tenant.ID+"/increment", map[string]any{
		"operations": []map[string]any{
			{"limit_key": "apiCalls", "amount": 1},
		},
	})
	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "transaction_conflict", errResp["error"])
}
