# Upgrading ContextForge

This document provides guidance for upgrading between ContextForge versions.

## General Upgrade Process

1. **Review the changelog** for breaking changes between your current version and target version
2. **Backup your configuration** (HeaderPropagationPolicy resources, annotations)
3. **Update the Helm chart**:
   ```bash
   helm repo update
   helm upgrade contextforge contextforge/contextforge \
     --namespace contextforge-system \
     --reuse-values
   ```
4. **Verify the upgrade**:
   ```bash
   kubectl get pods -n contextforge-system
   kubectl logs -n contextforge-system deployment/contextforge-operator
   ```

## Rollback Procedure

If you encounter issues after upgrading:

```bash
# List revision history
helm history contextforge -n contextforge-system

# Rollback to previous version
helm rollback contextforge [REVISION] -n contextforge-system

# Verify rollback
kubectl get pods -n contextforge-system
```

## Version-Specific Notes

### v0.1.0 (Initial Release)

This is the initial release. No upgrade notes.

**Features:**
- Sidecar injection via mutating webhook
- Annotation-based header configuration
- HeaderPropagationPolicy CRD (status tracking only)

### v0.2.0 (Upcoming)

**Breaking Changes:**
- `HTTPS_PROXY` environment variable is no longer injected into application containers
  - **Migration:** If your application relied on `HTTPS_PROXY`, set it manually in your pod spec
  - **Reason:** The proxy only handles HTTP traffic; HTTPS uses CONNECT tunneling where headers cannot be propagated

**New Features:**
- Configurable HTTP server timeouts via environment variables
- Rate limiting middleware (opt-in)
- Prometheus metrics endpoint

**Configuration Changes:**
- New environment variables for proxy:
  - `READ_TIMEOUT` (default: 15s)
  - `WRITE_TIMEOUT` (default: 15s)
  - `IDLE_TIMEOUT` (default: 60s)
  - `READ_HEADER_TIMEOUT` (default: 5s)
  - `TARGET_DIAL_TIMEOUT` (default: 2s)

## Compatibility Matrix

| ContextForge Version | Kubernetes Version | Helm Version |
|---------------------|-------------------|--------------|
| 0.1.x               | 1.25+             | 3.10+        |
| 0.2.x               | 1.25+             | 3.10+        |

## Getting Help

If you encounter issues during upgrade:

1. Check the [GitHub Issues](https://github.com/bgruszka/contextforge/issues)
2. Review operator logs: `kubectl logs -n contextforge-system deployment/contextforge-operator`
3. Open a new issue with your upgrade scenario and error messages
