# Tierify Helm Chart

This Helm chart deploys Tierify, a backend limit engine for pricing plan tiers, on a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PersistentVolume provisioner support (if persistence is enabled)

## Installation

### Add Helm Repository (if published)

```bash
helm repo add tierify https://charts.tierify.example.com
helm repo update
```

### Install from local chart

```bash
# From the project root directory
helm install tierify ./helm/tierify

# With custom values
helm install tierify ./helm/tierify -f custom-values.yaml

# With specific namespace
helm install tierify ./helm/tierify --namespace tierify --create-namespace
```

### Install with custom values inline

```bash
helm install tierify ./helm/tierify \
  --set replicaCount=5 \
  --set image.tag=1.0.0 \
  --set persistence.size=20Gi
```

## Upgrading

```bash
# Upgrade to new version
helm upgrade tierify ./helm/tierify

# Upgrade with new values
helm upgrade tierify ./helm/tierify -f custom-values.yaml

# Force recreation of pods
helm upgrade tierify ./helm/tierify --force
```

## Uninstallation

```bash
# Uninstall release
helm uninstall tierify

# Uninstall and delete namespace
helm uninstall tierify --namespace tierify
kubectl delete namespace tierify
```

## Configuration

The following table lists the configurable parameters and their default values.

### Global Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Global Docker image registry | `""` |

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `tierify` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag | `latest` |
| `imagePullSecrets` | Image pull secrets | `[]` |

### Deployment Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `3` |
| `strategy.type` | Deployment strategy type | `RollingUpdate` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full name | `""` |

### Application Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.port` | HTTP server port | `8080` |
| `config.host` | HTTP server host | `0.0.0.0` |
| `config.readTimeout` | HTTP read timeout | `15s` |
| `config.writeTimeout` | HTTP write timeout | `15s` |
| `config.dbType` | Database type | `sqlite` |
| `config.dbConnectionString` | Database connection string | `/app/data/tierify.db` |
| `config.allowUsageBelowZero` | Allow negative usage | `false` |
| `config.log.level` | Log level (debug, info, warn, error) | `info` |
| `config.log.format` | Log format (json, text) | `json` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `80` |
| `service.targetPort` | Container target port | `8080` |
| `service.annotations` | Service annotations | `{}` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.className` | Ingress class name | `nginx` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.hosts` | Ingress hosts | See values.yaml |
| `ingress.tls` | Ingress TLS configuration | `[]` |

### Persistence Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `persistence.enabled` | Enable persistence | `true` |
| `persistence.storageClassName` | Storage class name | `""` |
| `persistence.accessModes` | Access modes | `[ReadWriteOnce]` |
| `persistence.size` | Volume size | `10Gi` |
| `persistence.existingClaim` | Use existing PVC | `""` |

### Resource Management

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Autoscaling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable autoscaling | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `3` |
| `autoscaling.maxReplicas` | Maximum replicas | `10` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |
| `autoscaling.targetMemoryUtilizationPercentage` | Target memory utilization | `80` |

### Health Checks

| Parameter | Description | Default |
|-----------|-------------|---------|
| `livenessProbe.enabled` | Enable liveness probe | `true` |
| `readinessProbe.enabled` | Enable readiness probe | `true` |
| `startupProbe.enabled` | Enable startup probe | `false` |

### Security

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` |
| `podSecurityContext.runAsNonRoot` | Run as non-root | `true` |
| `podSecurityContext.runAsUser` | User ID | `1000` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |

## Examples

### Minimal Production Setup

```yaml
# values-prod.yaml
replicaCount: 5

image:
  repository: registry.example.com/tierify
  tag: "1.0.0"
  pullPolicy: Always

persistence:
  enabled: true
  size: 20Gi
  storageClassName: fast-ssd

config:
  log:
    level: info
    format: json

resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 128Mi

autoscaling:
  enabled: true
  minReplicas: 5
  maxReplicas: 20
```

```bash
helm install tierify ./helm/tierify -f values-prod.yaml
```

### With Ingress and TLS

```yaml
# values-ingress.yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  hosts:
    - host: api.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: tierify-tls
      hosts:
        - api.example.com
```

```bash
helm install tierify ./helm/tierify -f values-ingress.yaml
```

### Development Setup

```yaml
# values-dev.yaml
replicaCount: 1

image:
  tag: dev
  pullPolicy: Always

persistence:
  enabled: false

config:
  log:
    level: debug
    format: text

resources:
  limits:
    cpu: 250m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 32Mi

autoscaling:
  enabled: false
```

```bash
helm install tierify-dev ./helm/tierify -f values-dev.yaml --namespace dev
```

### High Availability Setup

```yaml
# values-ha.yaml
replicaCount: 5

podDisruptionBudget:
  enabled: true
  minAvailable: 3

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - tierify
          topologyKey: kubernetes.io/hostname

autoscaling:
  enabled: true
  minReplicas: 5
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70
```

```bash
helm install tierify ./helm/tierify -f values-ha.yaml
```

## Common Tasks

### View Release Status

```bash
helm status tierify
helm list
```

### Get Values

```bash
# Get all values
helm get values tierify

# Get all values including defaults
helm get values tierify --all
```

### Test Release

```bash
helm test tierify
```

### Rollback

```bash
# Rollback to previous revision
helm rollback tierify

# Rollback to specific revision
helm rollback tierify 2
```

### Debug

```bash
# Dry run to see generated manifests
helm install tierify ./helm/tierify --dry-run --debug

# Template only (no installation)
helm template tierify ./helm/tierify
```

### Monitor Deployment

```bash
# Watch pods
kubectl get pods -l app.kubernetes.io/name=tierify -w

# View logs
kubectl logs -l app.kubernetes.io/name=tierify -f

# Check events
kubectl get events --sort-by=.metadata.creationTimestamp
```

## Troubleshooting

### Pods not starting

```bash
# Check pod status
kubectl describe pod <pod-name>

# Check logs
kubectl logs <pod-name>

# Check events
kubectl get events
```

### PVC not binding

```bash
# Check PVC status
kubectl get pvc

# Check storage classes
kubectl get storageclass

# Check PV
kubectl get pv
```

### Service not accessible

```bash
# Check service
kubectl get svc tierify
kubectl describe svc tierify

# Check endpoints
kubectl get endpoints tierify

# Port forward for testing
kubectl port-forward svc/tierify 8080:80
curl http://localhost:8080/health
```

## Maintenance

### Backup Database

```bash
# Get pod name
POD=$(kubectl get pod -l app.kubernetes.io/name=tierify -o jsonpath="{.items[0].metadata.name}")

# Backup database
kubectl exec $POD -- sqlite3 /app/data/tierify.db ".backup /app/data/backup.db"

# Copy backup locally
kubectl cp $POD:/app/data/backup.db ./backup-$(date +%Y%m%d).db
```

### Update Configuration

```bash
# Edit values
vim custom-values.yaml

# Apply changes
helm upgrade tierify ./helm/tierify -f custom-values.yaml
```

## Support

For issues and questions:
- Documentation: [DEPLOYMENT.md](../../DEPLOYMENT.md)
- Getting Started: [GETTING_STARTED.md](../../GETTING_STARTED.md)
- GitHub: https://github.com/example/tierify/issues
