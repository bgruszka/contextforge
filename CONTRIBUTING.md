# Contributing to ContextForge

Thank you for your interest in contributing to ContextForge! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for everyone.

## License

ContextForge is licensed under the [Apache License 2.0](LICENSE). By contributing, you agree that your contributions will be licensed under the same license.

## Getting Started

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- kind (for local testing)
- Helm 3+

### Development Setup

1. **Fork and clone the repository**

   ```bash
   git clone https://github.com/YOUR_USERNAME/contextforge.git
   cd contextforge
   ```

2. **Install dependencies**

   ```bash
   go mod download
   ```

3. **Build the project**

   ```bash
   make build-all
   ```

4. **Run tests**

   ```bash
   # Unit tests
   make test

   # E2E tests (creates Kind cluster)
   make test-e2e
   ```

### Local Development with Kind

```bash
# Create a Kind cluster
kind create cluster --name ctxforge-dev

# Install CRDs
make install

# Run the operator locally
make run
```

## How to Contribute

### Reporting Issues

- Check existing issues before creating a new one
- Use issue templates when available
- Provide clear reproduction steps for bugs
- Include relevant logs and environment details

### Submitting Changes

1. **Create a feature branch**

   ```bash
   git checkout -b feature/amazing-feature
   ```

2. **Make your changes**

   - Follow the code style guidelines below
   - Add tests for new functionality
   - Update documentation as needed

3. **Run tests locally**

   ```bash
   make test
   make lint  # if available
   ```

4. **Commit your changes**

   Use [Conventional Commits](https://www.conventionalcommits.org/) format:

   ```bash
   git commit -m "feat: add amazing feature"
   git commit -m "fix: resolve header propagation issue"
   git commit -m "docs: update installation guide"
   ```

   **Commit types:**
   - `feat`: New feature
   - `fix`: Bug fix
   - `docs`: Documentation changes
   - `test`: Adding or updating tests
   - `refactor`: Code refactoring
   - `chore`: Maintenance tasks

5. **Push and create a Pull Request**

   ```bash
   git push origin feature/amazing-feature
   ```

   Then open a Pull Request on GitHub.

### Pull Request Guidelines

- Provide a clear description of the changes
- Reference related issues (e.g., "Fixes #123")
- Ensure all CI checks pass
- Keep PRs focused on a single concern
- Be responsive to review feedback

## Code Style

### Go Code

- Follow standard Go conventions and [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Use descriptive variable and function names
- Add comments for exported functions and types
- Handle errors explicitly

### Kubernetes Resources

- Use lowercase with hyphens for resource names
- Follow Kubernetes naming conventions
- Include appropriate labels and annotations

### Documentation

- Use clear, concise language
- Include code examples where helpful
- Keep README.md and docs in sync with code changes

## Project Structure

```
contextforge/
├── api/v1alpha1/           # CRD type definitions
├── cmd/
│   ├── proxy/              # Sidecar proxy binary
│   └── main.go             # Operator binary
├── internal/
│   ├── config/             # Configuration loading
│   ├── handler/            # HTTP proxy handler
│   ├── server/             # HTTP server
│   └── webhook/            # Admission webhook
├── deploy/
│   └── helm/contextforge/  # Helm chart
├── website/                # Documentation site
├── tests/e2e/              # E2E tests
├── Dockerfile.proxy        # Proxy image
└── Dockerfile.operator     # Operator image
```

## Testing

### Unit Tests

```bash
make test
```

### E2E Tests

E2E tests run against a real Kubernetes cluster (Kind):

```bash
make test-e2e
```

### Writing Tests

- Place unit tests next to the code they test (`*_test.go`)
- Place E2E tests in `tests/e2e/`
- Use table-driven tests where appropriate
- Mock external dependencies

## Questions?

- Open a [GitHub Discussion](https://github.com/bgruszka/contextforge/discussions) for questions
- Check existing [documentation](https://ctxforge.io/docs/)

Thank you for contributing!
