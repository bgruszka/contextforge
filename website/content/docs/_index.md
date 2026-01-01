---
title: Documentation
weight: 1
next: /docs/getting-started
---

Welcome to the ContextForge documentation. Learn how to install, configure, and use ContextForge for automatic HTTP header propagation in your Kubernetes clusters.

## What is ContextForge?

ContextForge is a Kubernetes operator that injects a lightweight sidecar proxy into your pods. This proxy automatically captures HTTP headers from incoming requests and propagates them to all outgoing HTTP calls — without requiring any code changes in your applications.

## Quick Navigation

{{< cards >}}
  {{< card link="getting-started" title="Getting Started" icon="play" subtitle="Install and configure ContextForge in 5 minutes" >}}
  {{< card link="installation" title="Installation" icon="download" subtitle="Detailed installation options and requirements" >}}
  {{< card link="configuration" title="Configuration" icon="cog" subtitle="Annotations, CRDs, and advanced settings" >}}
  {{< card link="how-it-works" title="How It Works" icon="academic-cap" subtitle="Architecture and technical deep-dive" >}}
  {{< card link="examples" title="Examples" icon="code" subtitle="Real-world use cases and code samples" >}}
{{< /cards >}}

## Key Features

- **Zero Code Changes** — Just add Kubernetes annotations
- **Lightweight** — ~10MB memory, <5ms latency per request
- **Framework Agnostic** — Works with any HTTP client in any language
- **Kubernetes Native** — Uses standard admission webhooks and CRDs
- **Production Ready** — Battle-tested with health checks and graceful shutdown
