# P2000-NFTY

Production-ready Go application that monitors Dutch P2000 emergency service notifications via WebSocket and forwards filtered messages to [ntfy](https://ntfy.sh/).

## Features

- **Real-time Monitoring**: Connects to P2000 WebSocket stream at `wss://p2000.riekeltbrands.nl/websocket`
- **Flexible Filtering**:
  - Forward all messages (default)
  - Or filter by exact capcode matching
- **Reliable Notifications**: Forward filtered alerts to ntfy with retry logic and exponential backoff
- **Production Ready**:
  - Automatic reconnection with exponential backoff
  - Graceful shutdown handling (SIGTERM/SIGINT)
  - Structured logging with zerolog
  - Prometheus metrics endpoint
  - Kubernetes health checks (liveness/readiness probes)
- **Priority Mapping**: Automatically maps P2000 function codes to ntfy priorities
  - Function A (urgent) → Priority 5
  - Function B (high) → Priority 4
  - Others → Priority 3 (default)
- **Small & Secure**: ~10MB Docker image with non-root user
- **Cloud Native**: Full Kubernetes support with manifests included

## Quick Start

### Local Development

1. **Clone and setup**:
```bash
git clone https://github.com/kaije/p2000-nfty.git
cd p2000-nfty
```

2. **Configure application**:
Edit `config.yaml`:
```yaml
# Forward all messages (default: true)
forward_all: true

# Or set to false and specify capcodes to filter
# forward_all: false
# capcodes:
#   - "001180000"  # Your capcode
#   - "001180001"  # Another capcode

ntfy:
  server: "https://ntfy.sh"
  topic: "your-topic-name"
```

Find capcodes at: [https://www.p2000-online.net/capcodes](https://www.p2000-online.net/capcodes)

3. **Install dependencies**:
```bash
make deps
```

4. **Run locally**:
```bash
make run
```

5. **Test notifications**:
- Subscribe to your ntfy topic: `https://ntfy.sh/your-topic-name`
- Wait for P2000 messages matching your capcodes

### Docker

1. **Build image**:
```bash
make docker-build
```

2. **Run container**:
```bash
docker run --rm -it \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/config/config.yaml:ro \
  ghcr.io/kaije/p2000-nfty:latest
```

3. **Check health**:
```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

### Kubernetes

1. **Customize configuration**:
Edit `kubernetes/configmap.yaml` with your capcodes and ntfy settings.

2. **Deploy**:
```bash
make deploy
```

3. **Check status**:
```bash
make status
make logs
```

4. **Remove**:
```bash
make undeploy
```

## Configuration

### Config File (`config.yaml`)

```yaml
# Forward all messages regardless of capcode (default: true)
forward_all: true

# List of capcodes to filter (only used when forward_all is false)
capcodes:
  - "001180000"
  - "001180001"
  - "002123456"

ntfy:
  server: "https://ntfy.sh"
  topic: "p2000-alerts"
  token: ""  # Optional: for private topics
```

**Configuration Options:**

- `forward_all`: When `true` (default), forwards all P2000 messages regardless of capcode. When `false`, only forwards messages matching configured capcodes.
- `capcodes`: List of capcodes to filter. Only used when `forward_all: false`.
- `ntfy.server`: URL of your ntfy server
- `ntfy.topic`: Topic name for notifications
- `ntfy.token`: Optional authentication token for private topics

### Environment Variables

Environment variables override config file settings:

| Variable | Description | Default |
|----------|-------------|---------|
| `CONFIG_PATH` | Path to config file | `config.yaml` |
| `FORWARD_ALL` | Forward all messages (true/false) | `true` |
| `NTFY_SERVER` | ntfy server URL | From config file |
| `NTFY_TOPIC` | ntfy topic name | From config file |
| `NTFY_TOKEN` | ntfy auth token | From config file |
| `SERVER_PORT` | HTTP server port | `8080` |

### Kubernetes ConfigMap

For Kubernetes deployments, edit `kubernetes/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: p2000-config
data:
  config.yaml: |
    forward_all: true
    capcodes:
      - "001180000"
    ntfy:
      server: "http://ntfy:80"  # Use internal ntfy service
      topic: "p2000-alerts"
```

For sensitive data (like tokens), use Secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: p2000-secrets
stringData:
  ntfy-token: "your-secret-token"
```

Then reference in deployment:
```yaml
env:
- name: NTFY_TOKEN
  valueFrom:
    secretKeyRef:
      name: p2000-secrets
      key: ntfy-token
```

## Architecture

### Project Structure

```
.
├── cmd/
│   └── p2000-forwarder/
│       └── main.go              # Application entrypoint
├── internal/
│   ├── config/
│   │   └── config.go            # Configuration handling
│   ├── filter/
│   │   └── capcode.go           # Capcode filtering logic
│   ├── metrics/
│   │   └── prometheus.go        # Prometheus metrics
│   ├── notifier/
│   │   └── ntfy.go              # ntfy.sh client
│   └── websocket/
│       └── client.go            # WebSocket client with reconnection
├── kubernetes/
│   ├── configmap.yaml           # P2000 forwarder configuration
│   ├── deployment.yaml          # P2000 forwarder deployment
│   ├── service.yaml             # P2000 forwarder service
│   ├── servicemonitor.yaml      # Prometheus ServiceMonitor
│   ├── ntfy-pvc.yaml            # ntfy persistent storage
│   ├── ntfy-configmap.yaml      # ntfy server configuration
│   ├── ntfy-deployment.yaml     # ntfy server deployment
│   ├── ntfy-service.yaml        # ntfy service
│   ├── ntfy-certificate.yaml    # TLS certificate (cert-manager)
│   ├── istio-gateway.yaml       # Istio ingress gateway
│   └── istio-virtualservice.yaml # Istio routing rules
├── Dockerfile                   # Multi-stage Docker build
├── Makefile                     # Build automation
├── config.yaml                  # Example configuration
└── go.mod                       # Go module definition
```

### Message Flow

1. **Receive**: WebSocket client receives P2000 message
2. **Filter**: Check if any capcode matches configured filters
3. **Forward**: If match found, send notification to ntfy
4. **Metrics**: Track all events in Prometheus metrics

### WebSocket Client

- Automatic reconnection with exponential backoff (1s → 2s → 4s → max 30s)
- Ping/pong keepalive every 30 seconds
- Graceful handling of connection drops
- Connection status monitoring

### Filtering

Two modes available:

1. **Forward All** (default): All P2000 messages are forwarded to ntfy
2. **Capcode Filtering**: Only messages matching configured capcodes are forwarded
   - **Exact match only**: No wildcards or partial matches
   - **Multiple capcodes**: Message forwarded if ANY capcode matches
   - Optimized lookup using hash map (O(1) complexity)

### Notification Delivery

- Retry logic: 3 attempts with exponential backoff
- Request timeout: 10 seconds
- Priority mapping based on P2000 function code
- Automatic emoji tags (🚨 for emergency)

## Monitoring

### Prometheus Metrics

Available at `http://localhost:8080/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `p2000_messages_received_total` | Counter | Total P2000 messages received |
| `p2000_messages_filtered_total` | Counter | Messages matching filters |
| `p2000_notifications_sent_total` | Counter | Successful notifications |
| `p2000_notifications_failed_total` | Counter | Failed notifications |
| `p2000_notification_duration_seconds` | Histogram | Notification send duration |
| `p2000_websocket_connected` | Gauge | Connection status (0/1) |

### Health Checks

Available at `http://localhost:8080/health`:

Returns `200 OK` if:
- WebSocket is connected
- Received message within last 5 minutes

Returns `503 Service Unavailable` otherwise.

### Kubernetes Probes

The deployment includes:

**Liveness Probe**:
- Checks if application is running
- Restarts pod if unhealthy

**Readiness Probe**:
- Checks if application can receive traffic
- Removes from load balancer if unhealthy

## Development

### Prerequisites

- Go 1.21 or higher
- Docker (for containerization)
- kubectl (for Kubernetes deployment)
- Make (optional, for automation)

### Building

```bash
# Download dependencies
make deps

# Format code
make fmt

# Run linters
make lint

# Run tests
make test

# Build binary
make build

# Build everything
make all
```

### Testing Locally

1. Start the application:
```bash
make run
```

2. In another terminal, check health:
```bash
curl http://localhost:8080/health
```

3. View metrics:
```bash
curl http://localhost:8080/metrics
```

4. Subscribe to your ntfy topic:
```bash
# Web: https://ntfy.sh/your-topic-name
# CLI:
curl -s https://ntfy.sh/your-topic-name/json
```

## Deployment

### Docker Registry

Push to your container registry:

```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Build and push
make docker-build
make docker-push
```

### Kubernetes

1. **Update image** in `kubernetes/deployment.yaml`:
```yaml
image: your-registry/p2000-nfty:latest
```

2. **Configure capcodes** in `kubernetes/configmap.yaml`

3. **Deploy**:
```bash
kubectl apply -f kubernetes/
```

4. **Verify deployment**:
```bash
kubectl get pods -l app=p2000-forwarder
kubectl logs -f deployment/p2000-forwarder
```

### Self-Hosted ntfy Server (Optional)

Instead of using the public ntfy.sh service, you can deploy your own ntfy server on Kubernetes with Istio ingress.

**Prerequisites:**
- Istio installed in your cluster
- cert-manager installed for TLS certificates
- DNS record pointing to your Istio ingress (e.g., `ntfy.nrkdci.net`)

**Deployment steps:**

1. **Deploy ntfy server**:
```bash
kubectl apply -f kubernetes/ntfy-pvc.yaml
kubectl apply -f kubernetes/ntfy-configmap.yaml
kubectl apply -f kubernetes/ntfy-deployment.yaml
kubectl apply -f kubernetes/ntfy-service.yaml
```

2. **Setup TLS certificate**:
```bash
kubectl apply -f kubernetes/ntfy-certificate.yaml
```

Wait for certificate to be issued:
```bash
kubectl get certificate -n istio-system ntfy-tls-cert
```

3. **Configure Istio ingress**:
```bash
kubectl apply -f kubernetes/istio-gateway.yaml
kubectl apply -f kubernetes/istio-virtualservice.yaml
```

4. **Create ntfy authentication** (exec into ntfy pod):
```bash
kubectl exec -it deployment/ntfy -- ntfy user add admin
kubectl exec -it deployment/ntfy -- ntfy user add p2000-forwarder
kubectl exec -it deployment/ntfy -- ntfy access p2000-forwarder p2000-alerts write-only
```

5. **Get access token**:
```bash
kubectl exec -it deployment/ntfy -- ntfy token add p2000-forwarder
```

6. **Update p2000-forwarder configuration**:
Edit `kubernetes/configmap.yaml` and add the token:
```yaml
ntfy:
  server: "http://ntfy:80"
  topic: "p2000-alerts"
  token: "tk_xxxxxxxxxxxxx"  # Token from step 5
```

Then apply and restart:
```bash
kubectl apply -f kubernetes/configmap.yaml
kubectl rollout restart deployment/p2000-forwarder
```

7. **Access your ntfy server**:
- Web UI: `https://ntfy.nrkdci.net`
- Subscribe to topic: `https://ntfy.nrkdci.net/p2000-alerts`

**Note:** The example uses `letsencrypt-staging` for testing. For production, change to `letsencrypt-prod` in `kubernetes/ntfy-certificate.yaml`.

### Prometheus Operator

If using Prometheus Operator, apply the ServiceMonitor:

```bash
kubectl apply -f kubernetes/servicemonitor.yaml
```

This automatically configures Prometheus to scrape metrics.

## Troubleshooting

### No messages received

1. Check WebSocket connection:
```bash
kubectl logs -f deployment/p2000-forwarder | grep websocket
```

2. Verify capcodes are correct (check [p2000-online.net](https://www.p2000-online.net/))

3. Check if messages are being filtered out:
```bash
curl http://localhost:8080/metrics | grep p2000_messages
```

### Notifications not arriving

1. Check ntfy topic is accessible:
```bash
curl -d "test" https://ntfy.sh/your-topic-name
```

2. Review notification errors:
```bash
kubectl logs -f deployment/p2000-forwarder | grep "failed to send"
```

3. Check metrics:
```bash
curl http://localhost:8080/metrics | grep notifications_failed
```

### Pod not starting

1. Check resource limits:
```bash
kubectl describe pod -l app=p2000-forwarder
```

2. View pod events:
```bash
kubectl get events --sort-by='.lastTimestamp'
```

3. Check configuration:
```bash
kubectl get configmap p2000-config -o yaml
```

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Run `make lint test`
6. Submit a pull request

## License

MIT License - see LICENSE file for details

## Resources

- [P2000 Online](https://www.p2000-online.net/) - Capcode database
- [ntfy Documentation](https://docs.ntfy.sh/) - ntfy.sh docs
- [P2000 WebSocket](https://p2000.riekeltbrands.nl/) - Data source
- [Prometheus Metrics](https://prometheus.io/docs/practices/naming/) - Metrics best practices

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing issues for solutions
- Review logs and metrics for diagnostics

---

Built with Go, designed for Kubernetes, monitored with Prometheus.
