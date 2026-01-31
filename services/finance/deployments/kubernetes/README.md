# Finance Service Kubernetes Manifests

This directory contains Kubernetes manifests for deploying the Finance Service.

## Files

| File | Description |
|------|-------------|
| `deployment.yaml` | Main deployment with rolling updates, health checks, and security |
| `service.yaml` | ClusterIP services for gRPC and HTTP, plus headless for gRPC LB |
| `configmap.yaml` | Non-sensitive configuration |
| `secret.yaml.template` | Template for secrets (DO NOT commit real secrets!) |
| `hpa.yaml` | HorizontalPodAutoscaler for auto-scaling |
| `rbac.yaml` | ServiceAccount and RBAC permissions |
| `networkpolicy.yaml` | Network isolation policies |
| `pdb.yaml` | PodDisruptionBudget for availability |

## Prerequisites

1. Create the namespace:
   ```bash
   kubectl create namespace finance
   ```

2. Create secrets (use sealed-secrets or external-secrets in production):
   ```bash
   # Copy and edit the template
   cp secret.yaml.template secret.yaml
   # Edit secret.yaml with actual values
   kubectl apply -f secret.yaml
   ```

## Deployment

### Apply all manifests:
```bash
kubectl apply -f .
```

### Or apply individually:
```bash
kubectl apply -f rbac.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml  # From your secure location
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f hpa.yaml
kubectl apply -f networkpolicy.yaml
kubectl apply -f pdb.yaml
```

## Verification

```bash
# Check pod status
kubectl get pods -n finance

# Check service
kubectl get svc -n finance

# Check logs
kubectl logs -n finance -l app=finance-service -f

# Port forward for testing
kubectl port-forward -n finance svc/finance-service 50051:50051 8080:8080
```

## Production Considerations

1. **Secrets Management**: Use sealed-secrets, external-secrets, or HashiCorp Vault
2. **TLS**: Enable TLS for gRPC and HTTP using cert-manager
3. **Ingress**: Add Ingress resource for external access
4. **Monitoring**: Ensure Prometheus can scrape `/metrics` endpoint
5. **Logging**: Configure log aggregation (Loki, ELK, etc.)
