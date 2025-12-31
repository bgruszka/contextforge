---
title: How It Works
weight: 4
---

This page explains the architecture and internals of ContextForge.

## Architecture Overview

ContextForge consists of two main components:

1. **Operator** — A Kubernetes controller that watches for pod creation and injects the sidecar
2. **Proxy Sidecar** — A lightweight HTTP proxy that handles header propagation

```mermaid
flowchart TB
    subgraph cluster["Kubernetes Cluster"]
        subgraph operator["ContextForge Operator"]
            webhook["MutatingAdmissionWebhook"]
            webhook --> |"Intercepts Pod creation"| check["Check ctxforge.io/enabled"]
            check --> |"If enabled"| inject["Inject sidecar + HTTP_PROXY"]
        end

        subgraph pod["Application Pod"]
            proxy["ContextForge Proxy<br/>:9090"]
            app["App Container<br/>:8080"]
            proxy --> |"Forward requests"| app
        end

        webhook -.-> |"Patches pod spec"| pod
    end

    style cluster fill:#1e293b,stroke:#6366f1,stroke-width:2px
    style operator fill:#312e81,stroke:#818cf8,stroke-width:2px
    style pod fill:#164e63,stroke:#22d3ee,stroke-width:2px
    style proxy fill:#4c1d95,stroke:#a78bfa
    style app fill:#0e7490,stroke:#67e8f9
```

## Sidecar Injection Flow

When you create a pod with the `ctxforge.io/enabled: "true"` annotation:

```mermaid
sequenceDiagram
    participant User
    participant API as Kubernetes API
    participant Webhook as ContextForge Webhook
    participant Pod

    User->>API: kubectl apply -f deployment.yaml
    API->>Webhook: Intercept Pod creation

    Note over Webhook: Check annotation:<br/>ctxforge.io/enabled=true
    Note over Webhook: Extract headers list
    Note over Webhook: Create sidecar spec
    Note over Webhook: Add HTTP_PROXY env vars

    Webhook->>API: Return JSON patch
    API->>Pod: Create Pod with sidecar

    Note over Pod: Pod running with:<br/>• App container<br/>• ContextForge proxy
```

## Request Flow

Here's how headers are propagated through a request:

### Incoming Request

```mermaid
sequenceDiagram
    participant Client as External Client
    participant Proxy as ContextForge Proxy<br/>:9090
    participant App as Your Application<br/>:8080

    Client->>Proxy: HTTP Request<br/>x-request-id: abc123<br/>x-tenant-id: tenant-1

    Note over Proxy: 1. Extract configured headers
    Note over Proxy: 2. Store in context.Context

    Proxy->>App: Forward request<br/>(headers preserved)

    Note over App: Process business logic

    App->>Proxy: Response
    Proxy->>Client: Response
```

### Outgoing Request

When your application makes an HTTP call to another service:

```mermaid
sequenceDiagram
    participant App as Your Application
    participant Proxy as ContextForge Proxy
    participant ServiceB as Service B

    Note over App: http.Get("http://service-b")<br/>HTTP_PROXY=localhost:9090

    App->>Proxy: Outgoing HTTP request

    Note over Proxy: 1. Intercept request
    Note over Proxy: 2. Retrieve headers from context
    Note over Proxy: 3. Inject headers:<br/>x-request-id: abc123<br/>x-tenant-id: tenant-1

    Proxy->>ServiceB: Request with injected headers

    ServiceB->>Proxy: Response
    Proxy->>App: Response
```

### Full Chain Propagation

```mermaid
flowchart LR
    subgraph podA["Pod A"]
        proxyA["Proxy"]
        appA["App A"]
    end

    subgraph podB["Pod B"]
        proxyB["Proxy"]
        appB["App B"]
    end

    subgraph podC["Pod C"]
        proxyC["Proxy"]
        appC["App C"]
    end

    Client["Client<br/>x-request-id: abc"] --> proxyA
    proxyA --> appA
    appA --> proxyA
    proxyA --> |"x-request-id: abc"| proxyB
    proxyB --> appB
    appB --> proxyB
    proxyB --> |"x-request-id: abc"| proxyC
    proxyC --> appC

    style podA fill:#312e81,stroke:#818cf8
    style podB fill:#312e81,stroke:#818cf8
    style podC fill:#312e81,stroke:#818cf8
```

## Header Storage

ContextForge uses Go's `context.Context` for thread-safe, request-scoped header storage:

```go
// Simplified implementation
type contextKey string
const ContextKeyHeaders contextKey = "ctxforge-headers"

// Store headers from incoming request
headers := extractHeaders(request, configuredHeaders)
ctx := context.WithValue(request.Context(), ContextKeyHeaders, headers)

// Retrieve headers for outgoing request
if stored := ctx.Value(ContextKeyHeaders); stored != nil {
    for key, value := range stored.(map[string]string) {
        outboundRequest.Header.Set(key, value)
    }
}
```

## HTTP_PROXY Approach

ContextForge leverages the standard `HTTP_PROXY` and `HTTPS_PROXY` environment variables:

```mermaid
flowchart LR
    subgraph pod["Your Pod"]
        app["Application"]
        proxy["ContextForge Proxy<br/>localhost:9090"]
        env["ENV: HTTP_PROXY=localhost:9090"]
    end

    app --> |"All HTTP requests<br/>go through proxy"| proxy
    proxy --> |"Headers injected"| external["External Services"]

    style pod fill:#1e293b,stroke:#6366f1
    style proxy fill:#4c1d95,stroke:#a78bfa
```

1. The operator sets these env vars to point to the sidecar proxy (`localhost:9090`)
2. Most HTTP clients automatically use these proxies for outgoing requests
3. The proxy intercepts outgoing calls and injects headers

{{% callout type="info" %}}
**Compatibility:** This approach works with most HTTP clients in Go, Python, Node.js, Java, Ruby, and other languages. Some clients may require explicit configuration to respect proxy env vars.
{{% /callout %}}

## Performance

ContextForge is designed for minimal overhead:

| Metric | Value |
|--------|-------|
| Memory per pod | ~10MB |
| CPU per pod | ~10m |
| Latency overhead | <5ms |
| Throughput impact | <1% |

## Health Checks

The proxy exposes health endpoints:

- `/healthz` — Liveness probe (always returns 200)
- `/ready` — Readiness probe (checks if target app is reachable)

## Security

- Runs as non-root user (UID 65532)
- Read-only root filesystem
- No privileged capabilities required
- TLS for webhook communication (cert-manager or self-signed)
