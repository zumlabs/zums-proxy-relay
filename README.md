# zums-proxy-relay

[![CI](https://github.com/zumlabs/zums-proxy-relay/actions/workflows/ci.yml/badge.svg)](https://github.com/zumlabs/zums-proxy-relay/actions/workflows/ci.yml)

A small zero-dependency HTTP CONNECT relay written in Go.

It exposes a generic nginx-looking public surface while the authenticated proxy path handles HTTP forwarding and HTTPS tunneling. The project is intentionally small: one binary, Go standard library only, table-driven tests, and GitHub Actions CI.

## Features

- **HTTP CONNECT tunnel** for HTTPS traffic.
- **Absolute-form HTTP forwarding** for plain HTTP proxy requests.
- **Basic proxy authentication** via `Proxy-Authorization`.
- **Constant-time credential comparison** using `crypto/subtle`.
- **No default credentials**. Missing `AUTH_USER` or `AUTH_PASS` makes the process exit.
- **nginx-style public surface** with `GET /`, `GET /health`, and nginx-like 404 responses.
- **Structured logs** with Go's `log/slog`.
- **Graceful shutdown** on `SIGINT` and `SIGTERM`.
- **Zero runtime dependencies**.

## Quick start

```bash
export AUTH_USER=relay-user
export AUTH_PASS=relay-pass

go run ./cmd/server
```

Expected log:

```json
{"time":"2026-01-01T00:00:00Z","level":"INFO","msg":"relay listening","addr":"0.0.0.0:8080","auth":"required"}
```

Public surface:

```bash
curl http://localhost:8080/
curl http://localhost:8080/health
```

Proxy usage:

```bash
curl -x http://relay-user:relay-pass@localhost:8080 http://example.com
curl -x http://relay-user:relay-pass@localhost:8080 https://example.com
```

Auth failure should return `407`:

```bash
curl -x http://wrong:wrong@localhost:8080 http://example.com -o /dev/null -w "%{http_code}\n"
```

## Configuration

| Variable | Default | Required | Purpose |
|---|---:|---:|---|
| `PORT` | `8080` | No | Listen port. Railway injects this automatically. |
| `HOST` | `0.0.0.0` | No | Bind address. |
| `AUTH_USER` | none | Yes | Proxy username. |
| `AUTH_PASS` | none | Yes | Proxy password. |
| `DEBUG` | `false` | No | Enables debug logs when set to `true`. |

## Local development

```bash
make fmt
make vet
make test
make build
```

Equivalent raw Go commands:

```bash
gofmt -s -w .
go vet ./...
go test -race ./...
go build ./...
```

## Deployment on Railway

1. Push this repository to GitHub.
2. Railway → New Project → Deploy from GitHub → select `zums-proxy-relay`.
3. Set variables:
   - `AUTH_USER`
   - `AUTH_PASS`
   - `DEBUG=false`
   - Railway injects `PORT` automatically.
4. For normal HTTP checks, the Railway web domain can hit `/` or `/health`.
5. For CONNECT tunneling, enable Railway TCP Proxy:
   - Service → Settings → Networking → TCP Proxy → Add Proxy
   - Application port: `8080`
   - Use the generated `<domain>:<port>` as the proxy endpoint.

The `.up.railway.app` HTTP domain is not the CONNECT endpoint. Use the TCP Proxy address for clients.

## Architecture

```text
client
  │
  ├─ GET / or /health ───────────────► nginx-style public response
  │
  ├─ GET http://host/path + auth ────► HTTP forwarder ───► upstream HTTP
  │
  └─ CONNECT host:443 + auth ────────► TCP tunnel ───────► upstream TLS
```

Code layout:

```text
cmd/server/main.go        process config, logging, graceful shutdown
internal/proxy/auth.go    proxy basic auth
internal/proxy/upstream.go authority parsing
internal/proxy/server.go  public surface, forward proxy, CONNECT tunnel
```

## Security notes

- Never commit real credentials.
- `AUTH_USER` and `AUTH_PASS` are required to prevent accidental open proxies.
- `Proxy-Authorization` is stripped before forwarding to upstream servers.
- Logs never include credentials or raw auth headers.
- The relay intentionally allows authenticated clients to connect to arbitrary hosts; treat credentials as production secrets.

## License

MIT
