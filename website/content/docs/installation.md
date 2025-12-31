---
title: Installation
weight: 2
---

This guide covers all installation options for ContextForge.

## Requirements

| Component | Minimum Version |
|-----------|-----------------|
| Kubernetes | 1.24+ |
| Helm | 3.0+ |
| cert-manager | 1.0+ (optional, for TLS) |

## Helm Installation

### Add the Repository

```bash
helm repo add contextforge https://ctxforge.io
helm repo update
```

### Basic Installation

```bash
helm install contextforge contextforge/contextforge \
  --namespace ctxforge-system \
  --create-namespace
```

### Installation with Custom Values

```bash
helm install contextforge contextforge/contextforge \
  --namespace ctxforge-system \
  --create-namespace \
  --set operator.replicas=2 \
  --set proxy.image.tag=v0.2.0 \
  --set webhook.failurePolicy=Ignore
```

### Using a Values File

Create a `values.yaml` file:

```yaml
operator:
  replicas: 2
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi

proxy:
  image:
    repository: ghcr.io/bgruszka/contextforge-proxy
    tag: latest
  resources:
    requests:
      cpu: 50m
      memory: 32Mi
    limits:
      cpu: 200m
      memory: 64Mi

webhook:
  failurePolicy: Fail
  certManager:
    enabled: true
```

Install with the values file:

```bash
helm install contextforge contextforge/contextforge \
  --namespace ctxforge-system \
  --create-namespace \
  -f values.yaml
```

## Upgrading

To upgrade to a newer version:

```bash
helm repo update
helm upgrade contextforge contextforge/contextforge \
  --namespace ctxforge-system
```

## Uninstalling

To remove ContextForge:

```bash
helm uninstall contextforge --namespace ctxforge-system
kubectl delete namespace ctxforge-system
```

{{% callout type="warning" %}}
Uninstalling ContextForge will not remove the sidecar from existing pods. You'll need to restart those pods after removing the annotations.
{{% /callout %}}

## Manual Installation (kubectl)

If you prefer not to use Helm, you can install using raw manifests:

```bash
# Install CRDs
kubectl apply -f https://raw.githubusercontent.com/bgruszka/contextforge/master/config/crd/bases/ctxforge.ctxforge.io_headerpropagationpolicies.yaml

# Install operator
kubectl apply -f https://raw.githubusercontent.com/bgruszka/contextforge/master/config/default/
```

## Verify Installation

Check the operator is running:

```bash
kubectl get pods -n ctxforge-system
kubectl get mutatingwebhookconfigurations | grep contextforge
```

Check the CRD is installed:

```bash
kubectl get crd headerpropagationpolicies.ctxforge.ctxforge.io
```

## Troubleshooting

### Pods Not Getting Sidecar Injected

1. **Check the namespace label:**
   ```bash
   kubectl get namespace <your-namespace> -o jsonpath='{.metadata.labels}'
   ```
   Ensure `ctxforge.io/injection` is not set to `disabled`.

2. **Check pod annotations:**
   ```bash
   kubectl get pod <pod-name> -o jsonpath='{.metadata.annotations}'
   ```
   Verify `ctxforge.io/enabled: "true"` is present.

3. **Check webhook logs:**
   ```bash
   kubectl logs -n ctxforge-system deployment/contextforge-operator
   ```

### Webhook Timeouts

If pod creation is slow, the webhook might be timing out:

```bash
# Check webhook configuration
kubectl get mutatingwebhookconfiguration contextforge-mutating-webhook -o yaml

# Increase timeout if needed (via Helm values)
webhook:
  timeoutSeconds: 30
```
