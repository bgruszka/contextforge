# ContextForge Configuration Reference

This document provides a complete reference for configuring ContextForge.

## Table of Contents

- [Pod Annotations](#pod-annotations)
- [Proxy Environment Variables](#proxy-environment-variables)
- [Helm Chart Values](#helm-chart-values)
- [HeaderPropagationPolicy CRD](#headerpropagationpolicy-crd)

---

## Pod Annotations

Add these annotations to your Pod spec to configure sidecar injection:

| Annotation | Required | Default | Description |
|------------|----------|---------|-------------|
| `ctxforge.io/enabled` | Yes | - | Set to `"true"` to enable sidecar injection |
| `ctxforge.io/headers` | Yes | - | Comma-separated list of headers to propagate |
| `ctxforge.io/target-port` | No | `8080` | Your application's listening port |

### Example

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
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

---

## Proxy Environment Variables

These environment variables configure the injected sidecar proxy. They can be set via Helm values or directly in the sidecar container.

### Core Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `HEADERS_TO_PROPAGATE` | (required*) | Comma-separated list of headers to propagate |
| `HEADER_RULES` | - | JSON array of advanced header rules (alternative to HEADERS_TO_PROPAGATE) |
| `TARGET_HOST` | `localhost:8080` | Target application host:port |
| `PROXY_PORT` | `9090` | Port the proxy listens on |
| `LOG_LEVEL` | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `console` | Log format: `console` (human-readable) or `json` |
| `METRICS_PORT` | `9091` | Port for Prometheus metrics (if separate from proxy) |

*Either `HEADERS_TO_PROPAGATE` or `HEADER_RULES` is required.

### Advanced Header Rules (HEADER_RULES)

For advanced configuration including header generation and path/method filtering, use `HEADER_RULES` with a JSON array:

```bash
HEADER_RULES='[
  {"name": "x-request-id", "generate": true, "generatorType": "uuid"},
  {"name": "x-tenant-id"},
  {"name": "x-api-key", "pathRegex": "^/api/.*", "methods": ["POST", "PUT"]}
]'
```

#### Header Rule Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | (required) | HTTP header name |
| `generate` | bool | `false` | Auto-generate if header is missing |
| `generatorType` | string | `uuid` | Generator: `uuid`, `ulid`, or `timestamp` |
| `propagate` | bool | `true` | Whether to propagate this header |
| `pathRegex` | string | - | Regex pattern to match request paths |
| `methods` | []string | - | HTTP methods to match (e.g., `["GET", "POST"]`) |

#### Generator Types

| Type | Format | Example |
|------|--------|---------|
| `uuid` | UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `ulid` | ULID (sortable) | `01ARZ3NDEKTSV4RRFFQ69G5FAV` |
| `timestamp` | RFC3339Nano | `2025-01-01T12:00:00.123456789Z` |

#### Example: Auto-generate Request ID

```bash
HEADER_RULES='[{"name":"x-request-id","generate":true,"generatorType":"uuid"}]'
```

#### Example: Path and Method Filtering

Only propagate headers for API endpoints, not health checks:

```bash
HEADER_RULES='[
  {"name":"x-request-id","generate":true,"generatorType":"uuid","pathRegex":"^/api/.*"},
  {"name":"x-tenant-id","pathRegex":"^/api/.*","methods":["POST","PUT","DELETE"]}
]'
```

### Timeout Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `READ_TIMEOUT` | `15s` | Max time to read entire request including body |
| `WRITE_TIMEOUT` | `15s` | Max time before timing out response writes |
| `IDLE_TIMEOUT` | `60s` | Max time to wait for next request (keep-alive) |
| `READ_HEADER_TIMEOUT` | `5s` | Max time to read request headers |
| `TARGET_DIAL_TIMEOUT` | `2s` | Timeout for connecting to target application |

Timeout values use Go duration format: `15s`, `1m30s`, `500ms`, etc.

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `false` | Enable rate limiting middleware |
| `RATE_LIMIT_RPS` | `1000` | Maximum requests per second |
| `RATE_LIMIT_BURST` | `100` | Maximum burst size (token bucket) |

When rate limit is exceeded, the proxy returns HTTP 429 (Too Many Requests).

### Example with Custom Timeouts

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  annotations:
    ctxforge.io/enabled: "true"
    ctxforge.io/headers: "x-request-id"
spec:
  containers:
    - name: app
      image: my-app:latest
      env:
        # Injected sidecar will use these if set
        - name: READ_TIMEOUT
          value: "30s"
        - name: WRITE_TIMEOUT
          value: "30s"
        - name: RATE_LIMIT_ENABLED
          value: "true"
        - name: RATE_LIMIT_RPS
          value: "500"
```

---

## Helm Chart Values

### Operator Configuration

```yaml
operator:
  # Number of operator replicas
  replicaCount: 1

  image:
    repository: ghcr.io/bgruszka/contextforge-operator
    tag: "0.1.0"
    pullPolicy: IfNotPresent

  # Resource requests/limits
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi

  # PodDisruptionBudget
  pdb:
    enabled: true
    minAvailable: 1

  # Leader election (required for HA)
  leaderElection:
    enabled: true

  # Metrics endpoint
  metrics:
    enabled: true
    port: 8080

  # Health probe port
  healthProbe:
    port: 8081
```

### Proxy Sidecar Configuration

```yaml
proxy:
  image:
    repository: ghcr.io/bgruszka/contextforge-proxy
    tag: "0.1.0"
    pullPolicy: IfNotPresent

  # Resource requests/limits for injected sidecar
  resources:
    requests:
      cpu: 25m
      memory: 32Mi
    limits:
      cpu: 200m
      memory: 128Mi

  # Default proxy port
  port: 9090

  # Default target port (application port)
  defaultTargetPort: 8080

  # Default log level
  logLevel: info
```

### Webhook Configuration

```yaml
webhook:
  # Webhook server port
  port: 9443

  # What to do if webhook fails: Fail or Ignore
  failurePolicy: Fail

  # Certificate configuration
  certManager:
    # Use cert-manager for webhook certificates
    enabled: false
    # Create a self-signed issuer
    createSelfSignedIssuer: true
    # Or use an existing issuer
    issuerRef:
      kind: Issuer
      name: my-issuer

  # Self-signed certificate settings (if cert-manager disabled)
  selfSigned:
    validityDays: 365
```

### Full Example

```yaml
# values.yaml
operator:
  replicaCount: 2
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
  pdb:
    enabled: true
    minAvailable: 1

proxy:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  logLevel: info

webhook:
  failurePolicy: Fail
  certManager:
    enabled: true
    createSelfSignedIssuer: true
```

---

## HeaderPropagationPolicy CRD

The HeaderPropagationPolicy CRD provides advanced header configuration.

### Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `podSelector` | LabelSelector | Selects pods to apply this policy (optional, matches all if empty) |
| `propagationRules` | []PropagationRule | List of header propagation rules |

### PropagationRule Fields

| Field | Type | Description |
|-------|------|-------------|
| `headers` | []HeaderConfig | Headers to propagate with this rule |
| `pathRegex` | string | Optional regex to match request paths |
| `methods` | []string | Optional list of HTTP methods to match |

### HeaderConfig Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | (required) | HTTP header name |
| `generate` | bool | `false` | Auto-generate if header is missing |
| `generatorType` | string | - | Generator type: `uuid`, `ulid`, `timestamp` |
| `propagate` | bool | `true` | Whether to propagate this header |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Current state conditions |
| `observedGeneration` | int64 | Last observed generation |
| `appliedToPods` | int32 | Number of pods this policy applies to |

### Example: Basic Policy

```yaml
apiVersion: ctxforge.ctxforge.io/v1alpha1
kind: HeaderPropagationPolicy
metadata:
  name: tracing-headers
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: my-service
  propagationRules:
    - headers:
        - name: x-request-id
        - name: x-correlation-id
        - name: x-tenant-id
```

### Example: Auto-Generate Request ID

```yaml
apiVersion: ctxforge.ctxforge.io/v1alpha1
kind: HeaderPropagationPolicy
metadata:
  name: auto-request-id
spec:
  propagationRules:
    - headers:
        - name: x-request-id
          generate: true
          generatorType: uuid
        - name: x-tenant-id
```

### Example: Path-Based Rules

```yaml
apiVersion: ctxforge.ctxforge.io/v1alpha1
kind: HeaderPropagationPolicy
metadata:
  name: api-headers
spec:
  propagationRules:
    # Propagate tenant ID only for /api/* paths
    - pathRegex: "^/api/.*"
      headers:
        - name: x-tenant-id
        - name: x-request-id
    # Propagate debug headers only for POST/PUT
    - methods: ["POST", "PUT"]
      headers:
        - name: x-debug-id
```

---

## Prometheus Metrics

The proxy exposes metrics at the `/metrics` endpoint.

### Available Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ctxforge_proxy_requests_total` | Counter | `method`, `status` | Total HTTP requests processed |
| `ctxforge_proxy_request_duration_seconds` | Histogram | `method` | Request duration distribution |
| `ctxforge_proxy_headers_propagated_total` | Counter | - | Total headers propagated |
| `ctxforge_proxy_active_connections` | Gauge | - | Current active connections |

### Example Prometheus Queries

```promql
# Request rate by status code
rate(ctxforge_proxy_requests_total[5m])

# 95th percentile latency
histogram_quantile(0.95, rate(ctxforge_proxy_request_duration_seconds_bucket[5m]))

# Error rate (5xx responses)
sum(rate(ctxforge_proxy_requests_total{status=~"5.."}[5m])) / sum(rate(ctxforge_proxy_requests_total[5m]))

# Headers propagated per second
rate(ctxforge_proxy_headers_propagated_total[5m])
```

### Grafana Dashboard

A sample Grafana dashboard is available at `deploy/grafana/contextforge-dashboard.json`.

---

## Health Endpoints

| Endpoint | Method | Success Code | Description |
|----------|--------|--------------|-------------|
| `/healthz` | GET | 200 | Liveness probe - proxy is running |
| `/ready` | GET | 200 | Readiness probe - target is reachable |
| `/metrics` | GET | 200 | Prometheus metrics |

### Kubernetes Probe Configuration

The injected sidecar automatically configures probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

## Troubleshooting

### Common Issues

**Sidecar not injected:**
- Ensure `ctxforge.io/enabled: "true"` annotation is set
- Check webhook is running: `kubectl get pods -n contextforge-system`
- Verify webhook certificate is valid

**Headers not propagating:**
- Enable debug logging: `LOG_LEVEL=debug`
- Check proxy logs: `kubectl logs <pod> -c ctxforge-proxy`
- Verify `HEADERS_TO_PROPAGATE` includes your headers

**High latency:**
- Check `ctxforge_proxy_request_duration_seconds` metrics
- Increase timeout values if needed
- Review rate limiting settings

**429 Too Many Requests:**
- Rate limiting is enabled and limit exceeded
- Increase `RATE_LIMIT_RPS` or `RATE_LIMIT_BURST`
- Or disable with `RATE_LIMIT_ENABLED=false`

### Debug Logging

Enable verbose logging to troubleshoot issues:

```yaml
env:
  - name: LOG_LEVEL
    value: "debug"
  - name: LOG_FORMAT
    value: "json"
```
