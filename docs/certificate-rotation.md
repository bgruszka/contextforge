# Certificate Rotation Guide

ContextForge uses TLS certificates for the webhook server to secure communication between the Kubernetes API server and the operator. This guide covers certificate management and rotation strategies.

## Certificate Options

ContextForge supports two certificate management approaches:

1. **cert-manager** (Recommended for production)
2. **Self-signed certificates** (Default, for development/testing)

## Using cert-manager (Recommended)

[cert-manager](https://cert-manager.io/) automatically handles certificate issuance, renewal, and rotation.

### Prerequisites

Install cert-manager:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
```

### Enable cert-manager in Helm

```yaml
# values.yaml
webhook:
  certManager:
    enabled: true
    createSelfSignedIssuer: true  # Creates a self-signed issuer
```

Or use an existing issuer:

```yaml
webhook:
  certManager:
    enabled: true
    createSelfSignedIssuer: false
    issuerRef:
      kind: ClusterIssuer
      name: letsencrypt-prod
```

### How It Works

1. cert-manager creates a `Certificate` resource
2. The certificate is automatically renewed before expiry (default: 30 days before)
3. The operator's certificate watcher detects file changes and reloads the TLS config
4. No manual intervention required

### Certificate Watcher

The operator includes a built-in certificate watcher that monitors for file changes:

```go
// From cmd/main.go
certWatcher, err := certwatcher.New(
    filepath.Join(certDir, "tls.crt"),
    filepath.Join(certDir, "tls.key"),
)
```

When cert-manager updates the certificate Secret, the operator automatically reloads without restart.

## Self-Signed Certificates (Default)

For development or testing, ContextForge can use self-signed certificates.

### Configuration

```yaml
# values.yaml
webhook:
  certManager:
    enabled: false
  selfSigned:
    validityDays: 365
```

### Manual Rotation

Self-signed certificates require manual rotation before expiry:

1. **Generate new certificates:**

```bash
# Generate CA
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -subj "/CN=contextforge-webhook-ca" -days 365 -out ca.crt

# Generate server certificate
openssl genrsa -out tls.key 2048
openssl req -new -key tls.key -subj "/CN=contextforge-webhook.contextforge-system.svc" -out server.csr

cat > server.ext << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = contextforge-webhook
DNS.2 = contextforge-webhook.contextforge-system
DNS.3 = contextforge-webhook.contextforge-system.svc
DNS.4 = contextforge-webhook.contextforge-system.svc.cluster.local
EOF

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 365 -extfile server.ext
```

2. **Update the Secret:**

```bash
kubectl create secret tls contextforge-webhook-certs \
  --cert=tls.crt \
  --key=tls.key \
  --dry-run=client -o yaml | kubectl apply -f -
```

3. **Update the webhook CA bundle:**

```bash
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')
kubectl patch mutatingwebhookconfiguration contextforge-mutating-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]"
```

4. **Restart the operator to reload certificates:**

```bash
kubectl rollout restart deployment contextforge-operator -n contextforge-system
```

## Monitoring Certificate Expiry

### Check Current Certificate

```bash
# View certificate details
kubectl get secret contextforge-webhook-certs -n contextforge-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check expiry
kubectl get secret contextforge-webhook-certs -n contextforge-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -enddate
```

### Prometheus Alerts

If using Prometheus, add an alert for certificate expiry:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: contextforge-cert-expiry
spec:
  groups:
    - name: contextforge
      rules:
        - alert: ContextForgeCertificateExpiringSoon
          expr: |
            (
              certmanager_certificate_expiration_timestamp_seconds{name="contextforge-webhook-cert"}
              - time()
            ) < 86400 * 14
          for: 1h
          labels:
            severity: warning
          annotations:
            summary: "ContextForge webhook certificate expiring soon"
            description: "Certificate will expire in less than 14 days"
```

## Troubleshooting

### Webhook Failures After Certificate Rotation

If pods fail to create after certificate rotation:

1. **Check webhook logs:**
```bash
kubectl logs -n contextforge-system deployment/contextforge-operator
```

2. **Verify certificate is valid:**
```bash
kubectl get secret contextforge-webhook-certs -n contextforge-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -text
```

3. **Check CA bundle matches:**
```bash
kubectl get mutatingwebhookconfiguration contextforge-mutating-webhook -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d | openssl x509 -noout -subject
```

### Certificate Mismatch

If the CA bundle doesn't match the certificate:

```bash
# Get the CA from the current certificate
kubectl get secret contextforge-webhook-certs -n contextforge-system -o jsonpath='{.data.ca\.crt}' | base64 -d > current-ca.crt

# Update webhook configuration
CA_BUNDLE=$(cat current-ca.crt | base64 | tr -d '\n')
kubectl patch mutatingwebhookconfiguration contextforge-mutating-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]"
```

## Best Practices

1. **Use cert-manager in production** - Automatic rotation eliminates manual intervention
2. **Monitor certificate expiry** - Set up alerts at least 14 days before expiry
3. **Test rotation in staging** - Verify the process works before production
4. **Document your rotation procedure** - Keep runbooks updated
5. **Use short-lived certificates** - Reduces blast radius if compromised (90 days recommended)
