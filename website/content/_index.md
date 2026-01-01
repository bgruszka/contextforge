---
title: ContextForge
layout: hextra-home
---

<div class="hx-mt-6"></div>

{{< hextra/hero-badge link="https://github.com/bgruszka/contextforge" >}}
  <span>Open Source</span>
  {{< icon name="github" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-8 hx-mb-8">
{{< hextra/hero-headline >}}
  Zero-Code Header Propagation&nbsp;<br class="sm:hx-block hx-hidden" />for Kubernetes
{{< /hextra/hero-headline >}}
</div>

{{< hextra/hero-subtitle >}}
  Automatically propagate HTTP headers like x-request-id, x-tenant-id,&nbsp;<br class="sm:hx-block hx-hidden" />through your microservice chain — no code changes required.
{{< /hextra/hero-subtitle >}}

<div style="height: 2.5rem;"></div>

<div style="margin-bottom: 4rem;">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
{{< hextra/hero-button text="GitHub" link="https://github.com/bgruszka/contextforge" style="background: #24292e; margin-left: 12px;" >}}
</div>

<div class="hx-mt-16"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Zero Code Changes"
    subtitle="Just add Kubernetes annotations to your pods. No SDK, no library, no code modifications needed."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(79,70,229,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Lightweight Proxy"
    subtitle="Only ~10MB memory and less than 5ms latency overhead per request. Minimal impact on your workloads."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(34,211,238,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Framework Agnostic"
    subtitle="Works with any language: Go, Python, Node.js, Java, Ruby, and more. Uses standard HTTP_PROXY."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(16,185,129,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Multi-Tenant Ready"
    subtitle="Propagate tenant IDs through all services for data isolation and audit logging in SaaS applications."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(245,158,11,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Request Tracing"
    subtitle="Track requests across services with correlation IDs. Debug issues by following the entire request chain (also works great with Telepresence for local development)."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(239,68,68,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Kubernetes Native"
    subtitle="Uses standard admission webhooks and CRDs. Production ready with health checks and graceful shutdown."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%,rgba(168,85,247,0.15),hsla(0,0%,100%,0));"
  >}}
{{< /hextra/feature-grid >}}

<div class="hx-mt-24"></div>

---

<div style="height: 1rem;"></div>

<h2 style="font-size: 2rem; font-weight: 700; margin-bottom: 1.5rem;">The Problem</h2>

<p style="font-size: 1.1rem; line-height: 1.8; margin-bottom: 1.5rem;">
Modern microservices rely on HTTP headers for <strong>request tracing</strong>, <strong>multi-tenancy</strong>, and <strong>debugging</strong>. Headers like <code>x-request-id</code>, <code>x-tenant-id</code>, and <code>x-correlation-id</code> must flow through every service.
</p>

<p style="font-size: 1.1rem; line-height: 1.8;">
<strong>But service meshes don't help.</strong> Istio, Linkerd, and Consul don't automatically propagate these headers. Your application code must manually extract incoming headers and attach them to every outgoing request.
</p>

<div style="height: 5rem;"></div>

---

<div style="height: 1rem;"></div>

<h2 style="font-size: 2rem; font-weight: 700; margin-bottom: 1.5rem;">Quick Start</h2>

<div style="height: 1rem;"></div>

**1. Install ContextForge:**

```bash
helm repo add contextforge https://ctxforge.io
helm install contextforge contextforge/contextforge -n ctxforge-system --create-namespace
```

<div class="hx-mt-8"></div>

**2. Annotate your pods:**

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
    spec:
      containers:
        - name: app
          image: my-app:latest
```

<div class="hx-mt-8"></div>

**3. Done!** Headers flow automatically through your service chain.

<div style="height: 5rem;"></div>

---

<div style="height: 1rem;"></div>

<h2 style="font-size: 2rem; font-weight: 700; margin-bottom: 1.5rem;">How It Works</h2>

<div style="height: 1rem;"></div>

<div class="mermaid-container" style="width: 100%; max-width: 1000px; margin: 0 auto;">

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'fontSize': '16px'}}}%%
flowchart TB
    subgraph pod["☸️ Your Kubernetes Pod"]
        direction TB

        req["📥 Incoming Request<br/>x-request-id: abc123<br/>x-tenant-id: acme"]

        subgraph proxy["🔄 ContextForge Proxy"]
            p1["1. Extract headers"]
            p2["2. Store in context"]
            p1 --> p2
        end

        subgraph app["🚀 Your Application"]
            a1["Makes HTTP call to another service"]
        end

        out["📤 Outgoing Request<br/>x-request-id: abc123 ✓<br/>x-tenant-id: acme ✓<br/>Headers auto-injected!"]

        req --> proxy
        proxy --> app
        app --> out
    end

    style pod fill:#1e293b,stroke:#6366f1,stroke-width:3px,color:#fff
    style proxy fill:#312e81,stroke:#818cf8,stroke-width:2px,color:#fff
    style app fill:#164e63,stroke:#22d3ee,stroke-width:2px,color:#fff
    style req fill:#1e3a5f,stroke:#60a5fa,stroke-width:2px,color:#fff
    style out fill:#14532d,stroke:#4ade80,stroke-width:2px,color:#fff
    style p1 fill:#4c1d95,stroke:#a78bfa,color:#fff
    style p2 fill:#4c1d95,stroke:#a78bfa,color:#fff
    style a1 fill:#0e7490,stroke:#67e8f9,color:#fff
```

</div>

<div style="height: 5rem;"></div>

---

<div style="height: 1rem;"></div>

<h2 style="font-size: 2rem; font-weight: 700; margin-bottom: 2rem;">Get Started</h2>

{{< cards >}}
  {{< card link="docs/getting-started" title="Get Started" icon="play" subtitle="Install ContextForge in 5 minutes" >}}
  {{< card link="docs/how-it-works" title="How It Works" icon="academic-cap" subtitle="Understand the architecture" >}}
  {{< card link="https://github.com/bgruszka/contextforge" title="GitHub" icon="github" subtitle="Star us and contribute" >}}
{{< /cards >}}

<div class="hx-mt-16"></div>
