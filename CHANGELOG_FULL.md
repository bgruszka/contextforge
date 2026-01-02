
### Bug Fixes

- Resolve golangci-lint issues (7c25e04)
- Update webhook tests for injectSidecar signature change (1498e0c)
- Add required propagationRules to controller test (ab4ce1b)
- Add cert-manager Certificate template and fix E2E workflow (6cd8566)
- Add --create-namespace flag to Helm install in E2E workflow (1cac392)
- Disable chart namespace creation to avoid conflict with --create-namespace (2a9a199)
- Remove unsupported --webhook-port flag from operator deployment (2113204)
- Remove objectSelector from webhook to allow annotation-based injection (94cd22f)
- Update Alpine base image to 3.21 (#23) (493fef5)
- Improve webhook security and resource allocation (#18, #19, #20, #22) (9979e62)
- Address golangci-lint issues (3f46232)
- Correct API group in RBAC template (c21fdfa)
- Address critical code review findings (#3, #16, #22, #26) (1f7670c)
- Route e2e test services through proxy port (9090) (12db349)
- Release Helm chart at tag time, not after PR merge (0cfa9cf)
- Upgrade git-cliff-action from v3 to v4 in release workflow (34c37a0)

### CI/CD

- Add GitHub Actions workflows (c5b620f)
- Remove duplicate workflow files (5cd5964)
- Update golangci-lint action to v6 for v2 config support (dafd7a9)
- Update golangci-lint action to v7 for v2 support (f1af05e)
- Use make test to setup envtest binaries (c07b8fa)
- Add Trivy vulnerability scanning (#12) (0a606dd)

### Documentation

- Add documentation and website (08535a2)
- Add comprehensive documentation and upgrade guide (#13, #14) (dece371)
- Add ctxforge.io/header-rules annotation documentation (68fab3c)
- Add certificate rotation documentation to website (db20619)

### Features

- Add HeaderPropagationPolicy CRD definitions (ab4efe6)
- Implement operator with sidecar injection webhook (97ce5da)
- Implement HTTP proxy for header propagation (3e43284)
- Add Kubernetes manifests for operator deployment (98d8392)
- Add Helm chart for ContextForge installation (c295b4d)
- Add Prometheus metrics package (#10) (84e4f73)
- Add rate limiting middleware (#24) (3237837)
- Implement controller reconcile loop (#17) (8477868)
- Add PodDisruptionBudget and improve Helm values (#11, #18, #22, #25) (3e5ef37)
- Add configurable timeouts and integrate rate limiting (#16, #24, #15) (0d8ff42)
- Add header generation, path/method filtering, and documentation (42ea966)
- Add ctxforge.io/header-rules annotation support in webhook (4118a1d)

### Miscellaneous

- Initialize Go module and build configuration (9235661)
- Add development environment configuration (65f1f30)
- Add Pod RBAC permissions for controller (#17) (1262170)

### Refactoring

- Improve error handling and add metrics recording (#21, #10) (2acc203)

### Testing

- Add e2e tests for header propagation (b8302d1)
- Add Keep-Alive context isolation tests for Issue #29 (ea25e9b)

### Build

- Add Docker configuration for operator and proxy (556be23)


