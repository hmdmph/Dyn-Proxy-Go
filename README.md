# Dyn-Proxy-Go

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)

A dynamic HTTP/HTTPS reverse proxy written in Go that intelligently routes requests based on the incoming `Host` header or HTTP/2 `:authority` pseudo-header. Features dynamic SNI support, transparent proxying, a built-in dashboard UI, and Kubernetes-ready deployment.

## Features

- **Dynamic Routing** — Automatically routes requests based on `Host` header (HTTP/1.1) or `:authority` pseudo-header (HTTP/2)
- **Dynamic SNI** — Automatic Server Name Indication based on request domain for proper TLS handshakes
- **Transparent Proxying** — Preserves original `Host` headers for backend servers
- **Dashboard UI** — Configurable HTML dashboard listing proxy endpoints
- **Structured Logging** — JSON-formatted logs with configurable levels and detailed request tracking
- **Health Checks** — Built-in `/health` endpoint for Kubernetes liveness/readiness probes
- **Graceful Shutdown** — Proper signal handling (`SIGINT`, `SIGTERM`) for clean shutdowns
- **Flexible Configuration** — CLI flags and environment variables
- **Distroless Docker Image** — Minimal, secure container image
- **Kubernetes Ready** — Complete K8s manifests included

## Quick Start

### Build & Run

```bash
# Build
make build

# Run with defaults (listens on :8080, proxies to port 443 via HTTPS)
./dyn-proxy-go

# Run with custom settings
./dyn-proxy-go -target-port=443 -target-scheme=https -log-level=debug

# Or use environment variables
export TARGET_PORT=443
export TARGET_SCHEME=https
export LOG_LEVEL=debug
./dyn-proxy-go
```

### Test the Proxy

```bash
# Route to httpbin.org via Host header
curl -H "Host: httpbin.org" http://localhost:8080/get

# Route to different backends dynamically
curl -H "Host: api.github.com" http://localhost:8080/zen
curl -H "Host: httpbin.org" http://localhost:8080/ip
```

### Docker

```bash
make docker-build
make docker-run

# With dev settings (debug logging, httpbin target)
make docker-run-dev
```

### Kubernetes

```bash
make k8s-deploy    # Deploy
make k8s-delete    # Teardown
```

## Configuration

All options can be set via CLI flags or environment variables. CLI flags take precedence.

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-port` | `LISTEN_PORT` | `8080` | Proxy listen port |
| `-target-host` | `TARGET_HOST` | `example.com` | Fallback target host |
| `-target-port` | `TARGET_PORT` | `443` | Target port to proxy to |
| `-target-scheme` | `TARGET_SCHEME` | `https` | Target scheme (`http` / `https`) |
| `-sni` | `SNI` | *(dynamic)* | SNI hostname (auto-set from request) |
| `-log-level` | `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `-skip-tls-verify` | `SKIP_TLS_VERIFY` | `false` | Skip TLS certificate verification |
| `-enable-tls` | `ENABLE_TLS` | `false` | Enable TLS for the proxy itself |
| `-tls-cert` | `TLS_CERT_FILE` | `/etc/certs/tls.crt` | TLS certificate file path |
| `-tls-key` | `TLS_KEY_FILE` | `/etc/certs/tls.key` | TLS private key file path |
| `-read-timeout` | `READ_TIMEOUT` | `30` | Read timeout (seconds) |
| `-write-timeout` | `WRITE_TIMEOUT` | `30` | Write timeout (seconds) |
| `-idle-timeout` | `IDLE_TIMEOUT` | `120` | Idle timeout (seconds) |
| `-page-title` | `PAGE_TITLE` | | Dashboard page title |
| `-sub-title` | `SUB_TITLE` | | Dashboard subtitle |
| `-page-gradient` | `PAGE_GRADIENT` | | Dashboard CSS gradient |
| `-page-title-icon` | `PAGE_TITLE_ICON` | | Dashboard title icon |
| `-proxy-list` | `PROXY_LIST` | | YAML proxy list for dashboard |

### Dynamic Routing Behavior

The proxy operates in **dynamic mode** by default:

1. Extracts the target domain from the incoming request's `Host` header or `:authority` pseudo-header
2. Routes requests to the extracted domain using the configured target port and scheme
3. Automatically sets SNI to match the request domain for proper TLS handshake
4. Preserves the original `Host` header for the backend server

### Dashboard Configuration

Configure the dashboard with a YAML proxy list via the `PROXY_LIST` environment variable:

```yaml
proxyList:
  - name: My API
    path: /api
  - name: HTTPBin
    path: /
```

## Health Check

```bash
curl http://localhost:8080/health
```

```json
{"status":"healthy","timestamp":"2026-04-05T10:30:00Z"}
```

## Structured Logging

JSON-formatted logs with detailed request tracking:

```json
{"time":"2026-04-05T10:30:00Z","level":"INFO","msg":"Starting dynamic proxy server","listen_port":8080,"target_port":443,"target_scheme":"https","mode":"dynamic_host_and_sni"}
{"time":"2026-04-05T10:30:01Z","level":"INFO","msg":"Request received","method":"GET","url":"/get","request_host":"httpbin.org","host_source":"Host"}
{"time":"2026-04-05T10:30:01Z","level":"INFO","msg":"Request completed","method":"GET","url":"/get","request_host":"httpbin.org","status_code":200,"duration_ms":150}
```

### Key Log Fields

- **`host_source`** — Whether domain came from `Host` header or `:authority` pseudo-header
- **`request_host`** — Domain extracted from the incoming request
- **`sni_hostname`** — Hostname used for SNI in TLS connections
- **`target_host`** — Target `host:port` used for the upstream connection

## Project Structure

```
.
├── main.go              # Entry point, signal handling, graceful shutdown
├── config.go            # Configuration parsing (flags + env vars)
├── server.go            # ProxyServer struct, Start/Stop, template loading
├── proxy.go             # Dynamic reverse proxy creation with SNI
├── handlers.go          # HTTP handlers (health, dashboard, proxy)
├── models.go            # Data models (ProxyEntry, ProxyListConfig)
├── proxy_test.go        # Tests
├── templates/
│   └── dashboard.html   # Embedded HTML template for dashboard UI
├── Dockerfile           # Multi-stage distroless build
├── Makefile             # Build, test, Docker, K8s commands
├── k8s/                 # Kubernetes manifests
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml
└── LICENSE              # MIT License
```

## Development

### Prerequisites

- Go 1.24+
- Docker / Podman (for containerization)
- kubectl (for Kubernetes deployment)

### Make Commands

```bash
make help            # Show all available commands
make build           # Build binary
make test            # Run tests
make test-coverage   # Run tests with coverage report
make fmt             # Format code
make vet             # Run go vet
make lint            # Run golangci-lint
make all             # Format, vet, test, and build
make release         # Cross-compile for linux/darwin/windows
```

## Kubernetes Deployment

The `k8s/` directory contains ready-to-use manifests. Customize the ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dyn-proxy-go-config
data:
  TARGET_HOST: "your-api.example.com"
  TARGET_PORT: "443"
  TARGET_SCHEME: "https"
  LOG_LEVEL: "info"
```

## Security

- **Distroless base image** — Minimal attack surface
- **Non-root user** — Runs as UID 65532
- **Read-only filesystem** — Container filesystem is read-only
- **No privilege escalation** — Prevented via security context
- **Resource limits** — CPU and memory limits configured

## License

This project is licensed under the [MIT License](LICENSE).
