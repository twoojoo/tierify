# Deployment Guide

This guide covers deploying Tierify in production using Docker and other deployment methods.

## Table of Contents

- [Docker Deployment](#docker-deployment)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Binary Deployment](#binary-deployment)
- [Configuration](#configuration)
- [Monitoring & Health Checks](#monitoring--health-checks)
- [Backup & Recovery](#backup--recovery)

---

## Docker Deployment

### Building the Image

```bash
# Build production image
docker build -t tierify:latest .

# Build with specific version tag
docker build -t tierify:1.0.0 .

# Build and tag for registry
docker build -t registry.example.com/tierify:latest .
```

### Running the Container

```bash
# Basic run
docker run -d \
  --name tierify \
  -p 8080:8080 \
  tierify:latest

# With environment variables
docker run -d \
  --name tierify \
  -p 8080:8080 \
  -e LOG_LEVEL=debug \
  -e ALLOW_USAGE_BELOW_ZERO=true \
  tierify:latest

# With persistent volume for database
docker run -d \
  --name tierify \
  -p 8080:8080 \
  -v tierify-data:/app/data \
  tierify:latest

# With custom database path
docker run -d \
  --name tierify \
  -p 8080:8080 \
  -e DB_CONNECTION_STRING=/app/data/production.db \
  -v /path/on/host:/app/data \
  tierify:latest
```

### Image Details

**Production Image:**
- Base: Alpine Linux 3.19 (minimal footprint)
- Size: ~20-30MB
- User: Non-root (tierify:tierify, UID 1000)
- Binary: Statically linked with SQLite
- Security: Stripped debug symbols, minimal attack surface

**Multi-stage Build:**
- Stage 1: Build with full Go toolchain
- Stage 2: Runtime with only essential libraries
- Result: Small, secure production image

---

## Docker Compose

### Production Deployment

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose down

# Stop and remove volumes (WARNING: deletes data)
docker-compose down -v
```

**docker-compose.yml** includes:
- Automatic restarts (`unless-stopped`)
- Health checks every 30s
- Persistent volume for database
- Isolated network
- Environment variable configuration

### Development with Hot Reload

```bash
# Start development environment
docker-compose -f docker-compose.dev.yml up

# With rebuild
docker-compose -f docker-compose.dev.yml up --build

# Stop
docker-compose -f docker-compose.dev.yml down
```

**docker-compose.dev.yml** features:
- Source code mounted as volume
- Air for hot reload
- Debug logging
- Go module cache
- Text format logs (easier to read)

### Environment Variables

Edit `docker-compose.yml` environment section:

```yaml
environment:
  - PORT=8080
  - HOST=0.0.0.0
  - DB_TYPE=sqlite
  - DB_CONNECTION_STRING=/app/data/tierify.db
  - LOG_LEVEL=info          # debug, info, warn, error
  - LOG_FORMAT=json         # json, text
  - ALLOW_USAGE_BELOW_ZERO=false
  - READ_TIMEOUT=15s
  - WRITE_TIMEOUT=15s
```

---

## Kubernetes

### Basic Deployment

Create `k8s/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tierify
  labels:
    app: tierify
spec:
  replicas: 3
  selector:
    matchLabels:
      app: tierify
  template:
    metadata:
      labels:
        app: tierify
    spec:
      containers:
      - name: tierify
        image: tierify:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: PORT
          value: "8080"
        - name: DB_TYPE
          value: "sqlite"
        - name: DB_CONNECTION_STRING
          value: "/app/data/tierify.db"
        - name: LOG_LEVEL
          value: "info"
        - name: LOG_FORMAT
          value: "json"
        volumeMounts:
        - name: data
          mountPath: /app/data
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: tierify-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: tierify
spec:
  selector:
    app: tierify
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
  type: LoadBalancer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: tierify-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

Deploy:

```bash
kubectl apply -f k8s/deployment.yaml
kubectl get pods -l app=tierify
kubectl logs -f deployment/tierify
```

### ConfigMap for Configuration

Create `k8s/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tierify-config
data:
  PORT: "8080"
  HOST: "0.0.0.0"
  DB_TYPE: "sqlite"
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
  ALLOW_USAGE_BELOW_ZERO: "false"
```

Reference in deployment:

```yaml
envFrom:
- configMapRef:
    name: tierify-config
```

---

## Binary Deployment

### Build Binary

```bash
# Build for current platform
make build

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o tierify-linux-amd64 ./cmd/tierify

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o tierify-linux-amd64 ./cmd/tierify
GOOS=linux GOARCH=arm64 go build -o tierify-linux-arm64 ./cmd/tierify
GOOS=darwin GOARCH=amd64 go build -o tierify-darwin-amd64 ./cmd/tierify
GOOS=windows GOARCH=amd64 go build -o tierify-windows-amd64.exe ./cmd/tierify
```

### Systemd Service

Create `/etc/systemd/system/tierify.service`:

```ini
[Unit]
Description=Tierify Limit Engine
After=network.target

[Service]
Type=simple
User=tierify
Group=tierify
WorkingDirectory=/opt/tierify
ExecStart=/opt/tierify/tierify
Restart=always
RestartSec=5

# Environment
Environment="PORT=8080"
Environment="DB_TYPE=sqlite"
Environment="DB_CONNECTION_STRING=/var/lib/tierify/tierify.db"
Environment="LOG_LEVEL=info"
Environment="LOG_FORMAT=json"

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/tierify

[Install]
WantedBy=multi-user.target
```

Setup and start:

```bash
# Create user
sudo useradd -r -s /bin/false tierify

# Create directories
sudo mkdir -p /opt/tierify /var/lib/tierify
sudo chown tierify:tierify /var/lib/tierify

# Copy binary
sudo cp tierify /opt/tierify/
sudo chown root:root /opt/tierify/tierify
sudo chmod 755 /opt/tierify/tierify

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable tierify
sudo systemctl start tierify

# Check status
sudo systemctl status tierify
sudo journalctl -u tierify -f
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `8080` | HTTP server port |
| `HOST` | No | `0.0.0.0` | HTTP server host |
| `READ_TIMEOUT` | No | `15s` | HTTP read timeout |
| `WRITE_TIMEOUT` | No | `15s` | HTTP write timeout |
| `DB_TYPE` | No | `sqlite` | Database type (only sqlite supported) |
| `DB_CONNECTION_STRING` | No | `tierify.db` | Database file path or connection string |
| `ALLOW_USAGE_BELOW_ZERO` | No | `false` | Allow negative usage values |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |
| `LOG_FORMAT` | No | `json` | Log format: json, text |

### Production Best Practices

**Logging:**
- Use `LOG_FORMAT=json` for production (structured logs)
- Use `LOG_LEVEL=info` or `warn` (avoid debug in production)
- Configure log rotation with your log management system

**Database:**
- Use persistent volumes for SQLite database
- Regular backups (see Backup & Recovery section)
- Monitor database size
- Consider connection pooling for future PostgreSQL support

**Security:**
- Run as non-root user (already configured in Docker)
- Use firewall to restrict access
- Enable TLS termination at reverse proxy (nginx, Traefik, etc.)
- Keep container/binary updated
- Use secrets management for sensitive config

**Performance:**
- Set appropriate resource limits (CPU, memory)
- Monitor /health endpoint
- Use multiple replicas for high availability
- Consider read replicas for scale (future PostgreSQL)

---

## Monitoring & Health Checks

### Health Check Endpoint

```bash
# Check health
curl http://localhost:8080/health

# Expected response
{"status":"ok"}
```

### Docker Health Check

Built-in health check in Dockerfile:
- Interval: 30s
- Timeout: 3s
- Start period: 5s (grace period)
- Retries: 3

View health status:
```bash
docker ps
# Look for "healthy" in STATUS column
```

### Kubernetes Probes

**Liveness Probe:** Restart container if unhealthy
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

**Readiness Probe:** Remove from load balancer if unhealthy
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Metrics & Observability

**Recommended monitoring:**
- Response times (p50, p95, p99)
- Request rates
- Error rates (4xx, 5xx)
- Database size
- Memory usage
- CPU usage
- Health check failures

**Future enhancements (not in MVP):**
- Prometheus metrics endpoint
- OpenTelemetry tracing
- Structured event logs

---

## Backup & Recovery

### SQLite Database Backup

**Manual backup:**
```bash
# Using Docker
docker exec tierify sqlite3 /app/data/tierify.db ".backup /app/data/backup-$(date +%Y%m%d).db"

# Copy backup out of container
docker cp tierify:/app/data/backup-20260213.db ./backup-20260213.db

# Using local binary
sqlite3 tierify.db ".backup backup-$(date +%Y%m%d).db"
```

**Automated backup script:**
```bash
#!/bin/bash
# backup-tierify.sh

CONTAINER_NAME="tierify"
BACKUP_DIR="/backups/tierify"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

docker exec $CONTAINER_NAME \
  sqlite3 /app/data/tierify.db ".backup /app/data/backup-$DATE.db"

docker cp "$CONTAINER_NAME:/app/data/backup-$DATE.db" "$BACKUP_DIR/"

# Keep only last 7 days of backups
find "$BACKUP_DIR" -name "backup-*.db" -mtime +7 -delete

echo "Backup completed: backup-$DATE.db"
```

**Cron job:**
```bash
# Daily backup at 2 AM
0 2 * * * /opt/scripts/backup-tierify.sh >> /var/log/tierify-backup.log 2>&1
```

### Restore from Backup

```bash
# Stop the service
docker-compose down

# Restore backup
docker run --rm -v tierify-data:/app/data -v $(pwd):/backup alpine \
  cp /backup/backup-20260213.db /app/data/tierify.db

# Start the service
docker-compose up -d
```

### Database Migration Strategy

When upgrading Tierify versions:

1. **Backup current database**
2. **Test migration on copy**
3. **Apply migration to production**
4. **Verify application startup**
5. **Keep backup for rollback**

**Rollback procedure:**
```bash
docker-compose down
docker run --rm -v tierify-data:/app/data -v $(pwd):/backup alpine \
  cp /backup/pre-migration-backup.db /app/data/tierify.db
docker-compose up -d
```

---

## Reverse Proxy Setup

### Nginx

```nginx
upstream tierify {
    server localhost:8080;
}

server {
    listen 80;
    server_name tierify.example.com;

    # Redirect to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tierify.example.com;

    ssl_certificate /etc/ssl/certs/tierify.crt;
    ssl_certificate_key /etc/ssl/private/tierify.key;

    location / {
        proxy_pass http://tierify;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /health {
        proxy_pass http://tierify/health;
        access_log off;
    }
}
```

### Traefik

```yaml
# docker-compose.yml with Traefik
version: '3.8'

services:
  tierify:
    image: tierify:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.tierify.rule=Host(`tierify.example.com`)"
      - "traefik.http.routers.tierify.entrypoints=websecure"
      - "traefik.http.routers.tierify.tls.certresolver=letsencrypt"
      - "traefik.http.services.tierify.loadbalancer.server.port=8080"
```

---

## Troubleshooting

### Container won't start

```bash
# Check logs
docker logs tierify

# Common issues:
# - Port already in use: Change port mapping
# - Database permission: Check volume permissions
# - Invalid config: Check environment variables
```

### Health check failing

```bash
# Manual health check
curl -v http://localhost:8080/health

# Check if service is listening
docker exec tierify netstat -tlnp | grep 8080

# Check logs for errors
docker logs tierify --tail 100
```

### Database locked

```bash
# SQLite database locked (multiple writers)
# Solution: Ensure only one instance accesses the DB
# For multi-instance: Use PostgreSQL (Phase 5)

# Check for lock file
docker exec tierify ls -la /app/data/tierify.db*
```

### Performance issues

```bash
# Check resource usage
docker stats tierify

# Check database size
docker exec tierify du -h /app/data/tierify.db

# Review slow query logs (if enabled)
docker logs tierify | grep -i "slow"
```

---

## Security Checklist

- [ ] Run as non-root user (✅ handled in Dockerfile)
- [ ] Use TLS/HTTPS via reverse proxy
- [ ] Restrict network access via firewall
- [ ] Use strong authentication in consuming apps
- [ ] Regular security updates
- [ ] Backup database regularly
- [ ] Monitor for unusual activity
- [ ] Use secrets management for env vars
- [ ] Scan Docker images for vulnerabilities
- [ ] Limit resource usage (CPU, memory)

---

## Support

For deployment issues:
- Check [GETTING_STARTED.md](GETTING_STARTED.md) for basics
- Review [README.md](README.md) for configuration
- Check Docker logs: `docker logs tierify`
- Verify health: `curl http://localhost:8080/health`
