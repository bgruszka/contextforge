---
title: Configuration
weight: 3
---

ContextForge can be configured through pod annotations and the HeaderPropagationPolicy CRD.

## Pod Annotations

### Required Annotations

| Annotation | Value | Description |
|------------|-------|-------------|
| `ctxforge.io/enabled` | `"true"` | Enables sidecar injection for this pod |

### Optional Annotations

| Annotation | Default | Description |
|------------|---------|-------------|
| `ctxforge.io/headers` | `""` | Comma-separated list of headers to propagate |
| `ctxforge.io/target-port` | `8080` | Port of your application container |

### Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  template:
    metadata:
      labels:
        ctxforge.io/enabled: "true"
      annotations:
        ctxforge.io/enabled: "true"
        ctxforge.io/headers: "x-request-id,x-tenant-id,x-correlation-id"
        ctxforge.io/target-port: "3000"
    spec:
      containers:
        - name: app
          image: my-app:latest
          ports:
            - containerPort: 3000
```

## HeaderPropagationPolicy CRD

For more advanced configuration, use the HeaderPropagationPolicy custom resource:

```yaml
apiVersion: ctxforge.ctxforge.io/v1alpha1
kind: HeaderPropagationPolicy
metadata:
  name: default-policy
  namespace: default
spec:
  selector:
    matchLabels:
      app: my-service

  propagationRules:
    - headers:
        - name: x-request-id
          generate: true          # Auto-generate if missing
          generatorType: uuid     # UUID generator
        - name: x-tenant-id
          propagate: true         # Always propagate
        - name: x-debug
          propagate: true
      pathRegex: ".*"             # Apply to all paths
      methods:                     # Apply to these methods
        - GET
        - POST
        - PUT
```

### CRD Fields

#### `spec.selector`

Selects which pods this policy applies to:

```yaml
selector:
  matchLabels:
    app: my-service
    environment: production
```

#### `spec.propagationRules`

List of rules defining which headers to propagate:

| Field | Type | Description |
|-------|------|-------------|
| `headers` | list | Headers to propagate |
| `headers[].name` | string | Header name (case-insensitive) |
| `headers[].generate` | bool | Generate header if missing |
| `headers[].generatorType` | string | Generator type: `uuid`, `timestamp` |
| `headers[].propagate` | bool | Whether to propagate (default: true) |
| `pathRegex` | string | Regex to match request paths |
| `methods` | list | HTTP methods to apply rule to |

## Proxy Environment Variables

The sidecar proxy is configured through environment variables (set automatically by the operator):

| Variable | Default | Description |
|----------|---------|-------------|
| `HEADERS_TO_PROPAGATE` | `""` | Comma-separated header names |
| `TARGET_HOST` | `localhost:8080` | Application container address |
| `PROXY_PORT` | `9090` | Proxy listen port |
| `LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `METRICS_PORT` | `9091` | Prometheus metrics port |

## Namespace Configuration

### Disable Injection for a Namespace

To prevent sidecar injection in a namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system
  labels:
    ctxforge.io/injection: disabled
```

### Enable Injection by Default

To inject sidecars into all pods in a namespace (without requiring annotations):

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    ctxforge.io/injection: enabled
```

{{% callout type="info" %}}
When namespace-level injection is enabled, you can still opt-out individual pods by setting `ctxforge.io/enabled: "false"` annotation.
{{% /callout %}}

## Helm Chart Values

Key configuration options in `values.yaml`:

```yaml
# Operator configuration
operator:
  replicas: 1
  image:
    repository: ghcr.io/bgruszka/contextforge-operator
    tag: latest
  resources:
    requests:
      cpu: 100m
      memory: 128Mi

# Proxy sidecar defaults
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

# Webhook configuration
webhook:
  failurePolicy: Fail  # or Ignore
  timeoutSeconds: 10
  certManager:
    enabled: false     # Set to true if using cert-manager

# RBAC
rbac:
  create: true

# Service Account
serviceAccount:
  create: true
  name: ""
```
