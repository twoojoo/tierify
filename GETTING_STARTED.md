# Getting Started with Tierify

This guide will walk you through running Tierify and making your first API calls.

## Step 1: Build and Run

```bash
# Build the project
make build

# Run the server
make run
```

The server starts on `http://localhost:8080` and creates a `tierify.db` SQLite database.

You should see output like:
```
{"level":"INFO","msg":"starting tierify","host":"0.0.0.0","port":8080}
{"level":"INFO","msg":"server listening","addr":"0.0.0.0:8080"}
```

## Step 2: Health Check

Verify the server is running:

```bash
curl http://localhost:8080/health
```

Response:
```json
{"status":"ok"}
```

## Step 3: Create Your First Tier

Create a "Professional" tier with an API call limit:

```bash
curl -X POST http://localhost:8080/api/v1/tiers \
  -H "Content-Type: application/json" \
  -d '{
    "key": "professional",
    "name": "Professional Plan",
    "limits": [
      {
        "key": "apiCalls",
        "kind": "absolute",
        "scope": "local",
        "config": {
          "max_value": 1000
        }
      },
      {
        "key": "storage",
        "kind": "cumulative",
        "scope": "local",
        "config": {
          "max_value": 10737418240,
          "unit": "bytes"
        }
      }
    ]
  }'
```

Response includes the tier ID, key, version (1), and timestamps.

## Step 4: Create a Tenant

Create a tenant and assign them to the "professional" tier:

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "customer-123",
    "tier_key": "professional"
  }'
```

Save the `id` from the response - you'll need it for the next steps.

## Step 5: Check Limits

Check if the tenant can make an API call:

```bash
TENANT_ID="<paste-tenant-id-here>"

curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/check \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 1}
    ]
  }'
```

Response shows the limit is available:
```json
{
  "results": [
    {
      "available": true,
      "limit_key": "apiCalls",
      "current_value": 0,
      "limit_value": 1000,
      "remaining": 1000,
      "limit_kind": "absolute"
    }
  ]
}
```

## Step 6: Increment Usage

Record an API call:

```bash
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/increment \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 1}
    ]
  }'
```

Now `current_value` is 1 and `remaining` is 999.

## Step 7: View Current Usage

Get all usage for the tenant:

```bash
curl http://localhost:8080/api/v1/tenants/$TENANT_ID/usage
```

Or get specific limit usage:

```bash
curl http://localhost:8080/api/v1/tenants/$TENANT_ID/usage/apiCalls
```

## Step 8: Try Transactions

Use a transaction for atomic operations:

```bash
# Start a transactional increment
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/increment \
  -H "Content-Type: application/json" \
  -H "X-Transaction-ID: tx-001" \
  -H "X-Transaction-TTL: 60" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 10}
    ]
  }'
```

The response includes transaction info with status "pending". Usage is NOT yet applied.

```bash
# Check transaction status
curl http://localhost:8080/api/v1/transactions/tx-001

# Commit the transaction (applies usage)
curl -X POST http://localhost:8080/api/v1/transactions/tx-001/commit

# Or rollback (discards operations)
# curl -X POST http://localhost:8080/api/v1/transactions/tx-001/rollback
```

## Step 9: Test Limit Exceeded

Try to exceed the limit:

```bash
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/increment \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"limit_key": "apiCalls", "amount": 1000}
    ]
  }'
```

You'll get a 429 status with:
```json
{
  "error": "limit_exceeded",
  "message": "one or more limits exceeded",
  "tenant_id": "...",
  "details": [
    {
      "limit_key": "apiCalls",
      "limit_kind": "absolute",
      "exceeded": true,
      "current_value": 11,
      "limit_value": 1000,
      "remaining": 989
    }
  ]
}
```

## Step 10: Manage Tenant Status

Block a tenant:

```bash
curl -X PUT http://localhost:8080/api/v1/tenants/$TENANT_ID/status \
  -H "Content-Type: application/json" \
  -d '{"status": "blocked"}'
```

Now all operations return 429 with `"blocked": true`.

Suspend a tenant (operations succeed but usage not tracked):

```bash
curl -X PUT http://localhost:8080/api/v1/tenants/$TENANT_ID/status \
  -H "Content-Type: application/json" \
  -d '{"status": "suspended"}'
```

Reactivate:

```bash
curl -X PUT http://localhost:8080/api/v1/tenants/$TENANT_ID/status \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
```

## Advanced: Tenant Hierarchy

Create an organization with global limits:

```bash
# Create org tier with global limit
curl -X POST http://localhost:8080/api/v1/tiers \
  -H "Content-Type: application/json" \
  -d '{
    "key": "organization",
    "name": "Organization",
    "limits": [
      {
        "key": "apiCalls",
        "kind": "absolute",
        "scope": "global",
        "config": {"max_value": 10000}
      }
    ]
  }'

# Create org tenant
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "acme-corp",
    "tier_key": "organization"
  }'

# Save ORG_ID from response

# Create user under org
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "user-alice",
    "parent_id": "'$ORG_ID'",
    "tier_key": "professional"
  }'
```

Now when the user increments `apiCalls`, it counts toward both:
- Their own local limit (1000)
- The org's global limit (10000)

## Common Operations

### Reset Usage

Reset specific limits:
```bash
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/reset \
  -H "Content-Type: application/json" \
  -d '{"limit_keys": ["apiCalls"]}'
```

Reset all limits:
```bash
curl -X POST http://localhost:8080/api/v1/tenants/$TENANT_ID/reset \
  -H "Content-Type: application/json" \
  -d '{}'
```

### Update Tier Version

```bash
# Create new version
curl -X PUT http://localhost:8080/api/v1/tiers/professional \
  -H "Content-Type: application/json" \
  -d '{
    "limits": [
      {
        "key": "apiCalls",
        "kind": "absolute",
        "scope": "local",
        "config": {"max_value": 2000}
      }
    ],
    "migrate_existing": true,
    "deprecate_previous": "latest"
  }'
```

### View Tier History

```bash
curl http://localhost:8080/api/v1/tenants/$TENANT_ID/history
```

## Development Workflow

```bash
# Run tests
make test

# Run with hot reload
make dev

# Format code
make fmt

# Run all checks
make check

# View coverage
make test-coverage
```

## Configuration

Create a `.env` file:

```bash
cp .env.example .env
```

Edit to customize:
```env
PORT=8080
DB_CONNECTION_STRING=tierify.db
LOG_LEVEL=debug
ALLOW_USAGE_BELOW_ZERO=false
```

## Next Steps

- Read [PLANNING.md](PLANNING.md) for detailed architecture
- Explore the API endpoints in [README.md](README.md)
- Check test files for more usage examples
- Integrate Tierify into your application

## Troubleshooting

**Port already in use:**
```bash
# Change port
PORT=9000 make run
```

**Database locked:**
```bash
# Stop all instances
pkill tierify
```

**Clean start:**
```bash
make clean
rm tierify.db
make run
```

## Support

For issues or questions, check:
- Test files for examples
- PLANNING.md for architecture details
- Handler tests for API usage patterns
