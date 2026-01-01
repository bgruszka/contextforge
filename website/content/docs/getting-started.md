---
title: Getting Started
weight: 1
---

Get ContextForge up and running in your Kubernetes cluster in just a few minutes.

## Prerequisites

Before you begin, ensure you have:

- **Kubernetes cluster** version 1.24 or later
- **Helm** version 3.0 or later
- **kubectl** configured to access your cluster
- **cluster-admin** permissions (for installing CRDs and webhooks)

## Installation

### Step 1: Add the Helm Repository

```bash
helm repo add contextforge https://ctxforge.io
helm repo update
```

### Step 2: Install ContextForge

```bash
helm install contextforge contextforge/contextforge \
  --namespace ctxforge-system \
  --create-namespace
```

### Step 3: Verify the Installation

Check that the operator is running:

```bash
kubectl get pods -n ctxforge-system
```

You should see output similar to:

```
NAME                                    READY   STATUS    RESTARTS   AGE
contextforge-operator-7b9f4d5c6-x2k8p   1/1     Running   0          30s
```

## Enable Header Propagation

To enable automatic header propagation for a pod, add the following annotations:

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
        ctxforge.io/headers: "x-request-id,x-tenant-id"
    spec:
      containers:
        - name: app
          image: my-app:latest
          ports:
            - containerPort: 8080
```

{{% details title="What do these annotations do?" closed="true" %}}

- **`ctxforge.io/enabled: "true"`** — Tells ContextForge to inject the sidecar proxy into this pod
- **`ctxforge.io/headers`** — Comma-separated list of headers to propagate

{{% /details %}}

## Verify It's Working

After deploying your annotated workload, verify the sidecar was injected:

```bash
kubectl get pod my-service-xxxxx -o jsonpath='{.spec.containers[*].name}'
```

You should see both your app container and the `ctxforge-proxy` container:

```
app ctxforge-proxy
```

## Test Header Propagation

Send a request with a custom header:

```bash
curl -H "x-request-id: test-123" http://your-service/api/endpoint
```

Check the logs of downstream services — they should all receive the `x-request-id` header!

## Next Steps

- [Installation Guide](../installation) — Detailed installation options
- [Configuration](../configuration) — All available annotations and settings
- [How It Works](../how-it-works) — Understand the architecture
- [Examples](../examples) — Real-world use cases
