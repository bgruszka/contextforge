#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CLUSTER_NAME="${CLUSTER_NAME:-ctxforge-e2e}"
PROXY_IMAGE="${PROXY_IMAGE:-contextforge-proxy:e2e}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-contextforge-operator:e2e}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    log "ERROR: $*" >&2
    exit 1
}

check_dependencies() {
    log "Checking dependencies..."

    command -v kind >/dev/null 2>&1 || error "kind is required but not installed"
    command -v kubectl >/dev/null 2>&1 || error "kubectl is required but not installed"
    command -v docker >/dev/null 2>&1 || error "docker is required but not installed"
    command -v helm >/dev/null 2>&1 || error "helm is required but not installed"

    log "All dependencies found"
}

create_cluster() {
    log "Creating kind cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log "Cluster ${CLUSTER_NAME} already exists"
        return 0
    fi

    kind create cluster \
        --name "${CLUSTER_NAME}" \
        --config "${SCRIPT_DIR}/kind-config.yaml" \
        --wait 120s

    log "Cluster created successfully"
}

build_images() {
    log "Building Docker images..."

    cd "${PROJECT_ROOT}"

    # Build proxy image
    log "Building proxy image: ${PROXY_IMAGE}"
    docker build -t "${PROXY_IMAGE}" -f Dockerfile.proxy .

    # Build operator image
    log "Building operator image: ${OPERATOR_IMAGE}"
    docker build -t "${OPERATOR_IMAGE}" -f Dockerfile.operator .

    log "Images built successfully"
}

load_images() {
    log "Loading images into kind cluster..."

    kind load docker-image "${PROXY_IMAGE}" --name "${CLUSTER_NAME}"
    kind load docker-image "${OPERATOR_IMAGE}" --name "${CLUSTER_NAME}"

    log "Images loaded successfully"
}

install_cert_manager() {
    log "Installing cert-manager..."

    # Check if cert-manager is already installed
    if kubectl get namespace cert-manager >/dev/null 2>&1; then
        log "cert-manager already installed"
        return 0
    fi

    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

    # Wait for cert-manager to be ready
    log "Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
    kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s

    log "cert-manager installed successfully"
}

deploy_operator() {
    log "Deploying ContextForge operator..."

    cd "${PROJECT_ROOT}"

    # Install CRDs
    kubectl apply -f config/crd/bases/

    # Deploy using Helm
    helm upgrade --install contextforge deploy/helm/contextforge \
        --namespace ctxforge-system \
        --create-namespace \
        --set operator.image.repository="${OPERATOR_IMAGE%%:*}" \
        --set operator.image.tag="${OPERATOR_IMAGE##*:}" \
        --set operator.image.pullPolicy=Never \
        --set proxy.image.repository="${PROXY_IMAGE%%:*}" \
        --set proxy.image.tag="${PROXY_IMAGE##*:}" \
        --set proxy.image.pullPolicy=Never \
        --set webhook.certManager.enabled=true \
        --wait \
        --timeout 180s

    log "Operator deployed successfully"
}

wait_for_webhook() {
    log "Waiting for webhook to be ready..."

    kubectl wait --for=condition=Available deployment/contextforge-operator \
        -n ctxforge-system \
        --timeout=120s

    # Give webhook a moment to register
    sleep 5

    log "Webhook is ready"
}

run_tests() {
    log "Running E2E tests..."

    cd "${PROJECT_ROOT}"

    # Set kubeconfig for tests
    export KUBECONFIG="${HOME}/.kube/config"

    go test -v ./tests/e2e/... -timeout 30m

    log "E2E tests completed"
}

cleanup() {
    log "Cleaning up..."

    if [[ "${SKIP_CLEANUP:-}" == "true" ]]; then
        log "Skipping cleanup (SKIP_CLEANUP=true)"
        return 0
    fi

    kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true

    log "Cleanup completed"
}

usage() {
    cat <<EOF
Usage: $0 [command]

Commands:
    setup       Create cluster, build and load images, deploy operator
    test        Run E2E tests (assumes setup is complete)
    all         Run setup and tests
    cleanup     Delete the kind cluster
    help        Show this help message

Environment variables:
    CLUSTER_NAME    Name of the kind cluster (default: ctxforge-e2e)
    PROXY_IMAGE     Proxy image name (default: contextforge-proxy:e2e)
    OPERATOR_IMAGE  Operator image name (default: contextforge-operator:e2e)
    SKIP_CLEANUP    Set to 'true' to skip cleanup after tests

Examples:
    $0 all                              # Full test run
    $0 setup                            # Just setup the environment
    SKIP_CLEANUP=true $0 all            # Keep cluster after tests
EOF
}

main() {
    local cmd="${1:-help}"

    case "${cmd}" in
        setup)
            check_dependencies
            create_cluster
            build_images
            load_images
            install_cert_manager
            deploy_operator
            wait_for_webhook
            ;;
        test)
            run_tests
            ;;
        all)
            check_dependencies
            create_cluster
            build_images
            load_images
            install_cert_manager
            deploy_operator
            wait_for_webhook
            run_tests
            cleanup
            ;;
        cleanup)
            cleanup
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            error "Unknown command: ${cmd}"
            ;;
    esac
}

main "$@"
