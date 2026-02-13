# Docker Quick Reference

Quick reference guide for Docker deployment of Tierify.

## Quick Start

```bash
# Build
docker build -t tierify:latest .

# Run
docker run -d --name tierify -p 8080:8080 -v tierify-data:/app/data tierify:latest

# Or use docker-compose
docker-compose up -d

# Check health
curl http://localhost:8080/health

# View logs
docker logs -f tierify
```

## Image Details

| Property | Value |
|----------|-------|
| **Base Image** | Alpine Linux 3.19 |
| **Size** | ~29MB |
| **Go Version** | Latest (1.23+) |
| **User** | Non-root (tierify:tierify, UID 1000) |
| **Database** | SQLite (embedded) |
| **Port** | 8080 |

## Make Commands

```bash
make docker-build          # Build production image
make docker-build-dev      # Build development image
make docker-run            # Run production container
make docker-run-dev        # Run dev container with hot reload
make docker-stop           # Stop containers
make docker-clean          # Remove images and volumes
make docker-compose-up     # Start with docker-compose
make docker-compose-down   # Stop docker-compose
make docker-compose-dev    # Dev environment with docker-compose
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `HOST` | `0.0.0.0` | HTTP server host |
| `DB_CONNECTION_STRING` | `/app/data/tierify.db` | Database file path |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `LOG_FORMAT` | `json` | Log format (json, text) |
| `ALLOW_USAGE_BELOW_ZERO` | `false` | Allow negative usage |

## Volumes

| Path | Purpose |
|------|---------|
| `/app/data` | SQLite database storage (persist this!) |
| `/app/migrations` | Database migrations (read-only) |

## Health Check

The image includes a built-in health check:
- **Endpoint**: `GET /health`
- **Interval**: 30s
- **Timeout**: 3s
- **Retries**: 3

## Security Features

✅ **Non-root user**: Runs as UID 1000 (tierify)
✅ **Minimal base**: Alpine Linux with only required packages
✅ **Statically linked**: No dynamic library dependencies
✅ **Stripped binary**: No debug symbols (-ldflags "-s -w")
✅ **Read-only filesystem**: Can run with read-only root

## Production Deployment

### With Docker

```bash
# Create volume for persistent data
docker volume create tierify-data

# Run with restart policy
docker run -d \
  --name tierify \
  --restart unless-stopped \
  -p 8080:8080 \
  -v tierify-data:/app/data \
  -e LOG_LEVEL=info \
  -e LOG_FORMAT=json \
  tierify:latest

# Behind nginx/traefik for TLS
docker run -d \
  --name tierify \
  --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v tierify-data:/app/data \
  tierify:latest
```

### With Docker Compose

```bash
# Production
docker-compose up -d

# Scale to multiple instances (requires load balancer)
docker-compose up -d --scale tierify=3
```

### With Kubernetes

See [DEPLOYMENT.md](DEPLOYMENT.md#kubernetes) for full K8s manifests.

```bash
kubectl apply -f k8s/
kubectl get pods -l app=tierify
```

## Development

### Hot Reload with Air

```bash
# Using Makefile
make docker-compose-dev

# Or directly
docker-compose -f docker-compose.dev.yml up

# Or standalone
make docker-run-dev
```

### Running Tests

```bash
# Run tests in Docker
docker run --rm tierify:latest go test ./...

# With coverage
docker run --rm tierify:latest go test -coverprofile=coverage.out ./...
```

## Backup & Restore

### Backup Database

```bash
# Backup from running container
docker exec tierify sqlite3 /app/data/tierify.db ".backup /app/data/backup.db"

# Copy out
docker cp tierify:/app/data/backup.db ./backup-$(date +%Y%m%d).db

# Automated with docker-compose
docker-compose exec tierify sqlite3 /app/data/tierify.db ".backup /app/data/backup.db"
```

### Restore Database

```bash
# Stop container
docker-compose down

# Restore backup to volume
docker run --rm -v tierify-data:/app/data -v $(pwd):/backup alpine \
  cp /backup/backup-20260213.db /app/data/tierify.db

# Start container
docker-compose up -d
```

## Troubleshooting

### Container exits immediately

```bash
# Check logs
docker logs tierify

# Common causes:
# - Port already in use
# - Invalid environment variables
# - Database permission issues
```

### Database locked errors

```bash
# Only one container can write to SQLite
# Ensure no other instances are running

docker ps -a | grep tierify
docker stop $(docker ps -aq --filter name=tierify)
```

### Permission denied errors

```bash
# Check volume permissions
docker run --rm -v tierify-data:/data alpine ls -la /data

# Fix if needed
docker run --rm -v tierify-data:/data alpine chown -R 1000:1000 /data
```

### High memory usage

```bash
# Set memory limits
docker run -d --name tierify --memory="256m" -p 8080:8080 tierify:latest

# Or in docker-compose.yml
services:
  tierify:
    deploy:
      resources:
        limits:
          memory: 256M
```

## Registry Push

```bash
# Tag for registry
docker tag tierify:latest registry.example.com/tierify:1.0.0
docker tag tierify:latest registry.example.com/tierify:latest

# Push
docker push registry.example.com/tierify:1.0.0
docker push registry.example.com/tierify:latest

# Or use Makefile
REGISTRY=registry.example.com VERSION=1.0.0 make docker-push
```

## Multi-Architecture Build

```bash
# Build for multiple platforms
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 -t tierify:latest .

# Build and push
docker buildx build --platform linux/amd64,linux/arm64 \
  -t registry.example.com/tierify:latest \
  --push .
```

## Further Reading

- Full deployment guide: [DEPLOYMENT.md](DEPLOYMENT.md)
- Getting started: [GETTING_STARTED.md](GETTING_STARTED.md)
- Configuration: [README.md](README.md#configuration)
