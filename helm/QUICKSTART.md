# Helm Quick Start Guide

Get Tierify running on Kubernetes in minutes with Helm.

## Prerequisites

- Kubernetes cluster (1.19+)
- Helm 3.0+
- `kubectl` configured

## Quick Install

```bash
# From project root
make helm-install

# Or directly with helm
helm install tierify ./helm/tierify --namespace tierify --create-namespace
```

## Verify Installation

```bash
# Check pod status
kubectl get pods -n tierify -l app.kubernetes.io/name=tierify

# Check service
kubectl get svc -n tierify

# Port forward to access locally
kubectl port-forward -n tierify svc/tierify 8080:80

# Test health endpoint
curl http://localhost:8080/health
```

## Common Configurations

### Development Setup

```bash
make helm-install-dev

# Or with helm
helm install tierify-dev ./helm/tierify \
  --namespace dev --create-namespace \
  --set replicaCount=1 \
  --set persistence.enabled=false \
  --set config.log.level=debug \
  --set config.log.format=text
```

### Production with Custom Settings

```bash
helm install tierify ./helm/tierify \
  --namespace production --create-namespace \
  --set replicaCount=5 \
  --set image.repository=your-registry.com/tierify \
  --set image.tag=1.0.0 \
  --set persistence.size=20Gi \
  --set resources.limits.memory=512Mi
```

### With Ingress and TLS

```bash
helm install tierify ./helm/tierify \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=tierify.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set ingress.tls[0].secretName=tierify-tls \
  --set ingress.tls[0].hosts[0]=tierify.example.com
```

### With Autoscaling

```bash
helm install tierify ./helm/tierify \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=3 \
  --set autoscaling.maxReplicas=10 \
  --set autoscaling.targetCPUUtilizationPercentage=70
```

## Using Values File

Create a `values-prod.yaml`:

```yaml
replicaCount: 5

image:
  repository: registry.example.com/tierify
  tag: "1.0.0"
  pullPolicy: Always

persistence:
  enabled: true
  size: 20Gi

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

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
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

Install with values file:

```bash
helm install tierify ./helm/tierify -f values-prod.yaml
```

## Management Commands

```bash
# Upgrade release
make helm-upgrade

# Check status
make helm-status

# Get deployed values
make helm-get-values

# Uninstall
make helm-uninstall

# List all releases
make helm-list
```

## Testing Before Deploy

```bash
# Dry run
make helm-dry-run

# Generate manifests to review
make helm-template > manifests.yaml
cat manifests.yaml

# Lint chart
make helm-lint
```

## Customize via Makefile Variables

```bash
# Install to custom namespace
HELM_NAMESPACE=staging make helm-install

# Use custom release name
HELM_RELEASE_NAME=my-tierify HELM_NAMESPACE=prod make helm-install
```

## Post-Install

After installation, Helm will display NOTES with:
- How to access the application
- Health check URL
- Commands to view logs
- Configuration summary

## Troubleshooting

### Pods not starting

```bash
kubectl describe pod -n tierify -l app.kubernetes.io/name=tierify
kubectl logs -n tierify -l app.kubernetes.io/name=tierify
```

### PVC not binding

```bash
kubectl get pvc -n tierify
kubectl describe pvc -n tierify
kubectl get storageclass
```

### Service not accessible

```bash
kubectl get svc -n tierify
kubectl describe svc tierify -n tierify
kubectl get endpoints tierify -n tierify
```

## Backup Database

```bash
# Get pod name
POD=$(kubectl get pod -n tierify -l app.kubernetes.io/name=tierify -o jsonpath="{.items[0].metadata.name}")

# Backup
kubectl exec -n tierify $POD -- sqlite3 /app/data/tierify.db ".backup /app/data/backup.db"

# Download backup
kubectl cp -n tierify $POD:/app/data/backup.db ./backup-$(date +%Y%m%d).db
```

## Next Steps

- Review full documentation: [helm/README.md](README.md)
- Customize values: [helm/tierify/values.yaml](tierify/values.yaml)
- Configure monitoring: Set `serviceMonitor.enabled=true` (requires Prometheus Operator)
- Enable network policies: Set `networkPolicy.enabled=true`

## All Available Values

See [values.yaml](tierify/values.yaml) for all configurable parameters including:
- Image settings (repository, tag, pull policy)
- Deployment settings (replicas, strategy, security)
- Service configuration (type, ports, annotations)
- Ingress settings (hosts, TLS, annotations)
- Persistence (size, storage class, access modes)
- Resources (CPU/memory limits and requests)
- Autoscaling (HPA configuration)
- Health checks (liveness, readiness, startup probes)
- Security (pod security context, service account)
- Monitoring (ServiceMonitor for Prometheus)
- Network policies
- And more!

## Help

```bash
# Show all Helm-related make commands
make helm-docs

# Show general help
make help
```
