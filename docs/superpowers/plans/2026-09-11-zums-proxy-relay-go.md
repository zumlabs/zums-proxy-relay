# zums-proxy-relay Go Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current Node.js proxy relay with an idiomatic, zero-dependency Go service named `zums-proxy-relay`, ready for GitHub portfolio and Railway deployment.

**Architecture:** Small Go service layout: `cmd/server` owns process startup/config/shutdown, `internal/proxy` owns all proxy behavior. No Gin, no DI framework, no third-party runtime dependencies.

**Tech Stack:** Go 1.25, standard library only (`net/http`, `net`, `crypto/subtle`, `log/slog`, `testing`, `httptest`), GitHub Actions CI, Railway deployment.

## Global Constraints

- Module path: `github.com/zumlabs/zums-proxy-relay`.
- Go version: use local `go1.25.5`; `go.mod` should declare `go 1.25`.
- Runtime dependencies: stdlib only; do not add third-party Go modules.
- Project layout: `cmd/server` + `internal/proxy`; no `pkg/`, no DI library.
- Auth: `AUTH_USER` and `AUTH_PASS` are required; empty values must fail startup.
- Auth compare: use `crypto/subtle.ConstantTimeCompare`, not `==`.
- Public surface: keep nginx disguise with `Server: nginx/1.24.0`.
- Health endpoint: `GET /health`, not `/healthz`.
- Proxy surface: absolute-form `http://...` forward and `CONNECT host:port` tunnel require `Proxy-Authorization`.
- Timeouts: upstream dial 5s; tunnel idle 30s; shutdown 10s; server read-header timeout 10s.
- Logging: use `log/slog`; never log credentials or raw auth headers.
- Testing: table-driven tests with named subtests; use stdlib `testing` and `httptest`.
- CI: run formatting check, `go vet`, `go build`, `go test -race`, and `golangci-lint`.
- Deployment: Railway with TCP Proxy for CONNECT; no custom CD workflow.
- Commands assume PowerShell from `D:\Portofolio\zums-proxy-relay` unless marked as GitHub Actions Linux.
- Commit after each task with conventional commit messages.

---

## File map

- Create: `go.mod` — Go module declaration.
- Delete: `server.js` — replaced by Go implementation.
- Delete: `package.json` — no Node runtime remains.
- Modify: `.gitignore` — Go, env, IDE, build artifacts.
- Create: `internal/proxy/upstream.go` — authority parsing.
- Create: `internal/proxy/upstream_test.go` — authority parsing executable spec.
- Create: `internal/proxy/auth.go` — Basic proxy auth.
- Create: `internal/proxy/auth_test.go` — auth executable spec.
- Create: `internal/proxy/server.go` — public surface, forward proxy, CONNECT tunnel.
- Create: `internal/proxy/server_test.go` — HTTP and CONNECT behavior tests.
- Create: `cmd/server/main.go` — env config, slog setup, graceful shutdown.
- Create: `cmd/server/main_test.go` — env config tests.
- Modify: `.env.example` — Go runtime env reference.
- Create: `Makefile` — common local commands.
- Create: `.golangci.yml` — minimal lint config.
- Create: `.github/workflows/ci.yml` — GitHub quality gate.
- Create: `Dockerfile` — portable container build, not required by Railway.
- Modify: `README.md` — public portfolio README.
- Create: `LICENSE` — MIT.

---

### Task 1: Scaffold Go module and remove Node runtime

**Files:**
- Create: `go.mod`
- Modify: `.gitignore`
- Delete: `server.js`
- Delete: `package.json`

**Interfaces:**
- Produces: Go module `github.com/zumlabs/zums-proxy-relay`.
- Produces: clean Go-only repo baseline for later tasks.

- [ ] **Step 1: Confirm Go version**

Run:

```powershell
go version
```

Expected:

```text
go version go1.25.5 windows/amd64
```

- [ ] **Step 2: Create `go.mod`**

Write `go.mod`:

```go
module github.com/zumlabs/zums-proxy-relay

go 1.25
```

- [ ] **Step 3: Replace `.gitignore`**

Write `.gitignore`:

```gitignore
.env
bin/
dist/
coverage.out
/vendor/
*.exe
*.test
.idea/
.vscode/
```

- [ ] **Step 4: Remove Node files**

Run:

```powershell
git rm server.js package.json
```

Expected:

```text
rm 'server.js'
rm 'package.json'
```

- [ ] **Step 5: Verify empty Go module is valid**

Run:

```powershell
go list ./...
```

Expected either empty output or no error.

- [ ] **Step 6: Commit**

Run:

```powershell
git add go.mod .gitignore
git commit -m "chore: initialize Go module"
```

---

### Task 2: Add authority parsing

**Files:**
- Create: `internal/proxy/upstream_test.go`
- Create: `internal/proxy/upstream.go`

**Interfaces:**
- Produces: `func splitAuthority(authority string, defaultPort int) (string, int, error)`.
- Used by: forward proxy default port 80 and CONNECT default port 443.

- [ ] **Step 1: Write failing tests**

Create `internal/proxy/upstream_test.go`:

```go
package proxy

import "testing"

func TestSplitAuthority(t *testing.T) {
	tests := []struct {
		name        string
		authority   string
		defaultPort int
		wantHost    string
		wantPort    int
		wantErr     bool
	}{
		{
			name:        "host without port uses default",
			authority:   "example.com",
			defaultPort: 80,
			wantHost:    "example.com",
			wantPort:    80,
		},
		{
			name:        "host with port",
			authority:   "example.com:8080",
			defaultPort: 80,
			wantHost:    "example.com",
			wantPort:    8080,
		},
		{
			name:        "ipv6 with port",
			authority:   "[::1]:8080",
			defaultPort: 443,
			wantHost:    "::1",
			wantPort:    8080,
		},
		{
			name:        "ipv6 without port uses default",
			authority:   "[::1]",
			defaultPort: 443,
			wantHost:    "::1",
			wantPort:    443,
		},
		{
			name:        "empty authority",
			authority:   "",
			defaultPort: 80,
			wantErr:     true,
		},
		{
			name:        "port without host",
			authority:   ":8080",
			defaultPort: 80,
			wantErr:     true,
		},
		{
			name:        "port zero",
			authority:   "example.com:0",
			defaultPort: 80,
			wantErr:     true,
		},
		{
			name:        "port too large",
			authority:   "example.com:65536",
			defaultPort: 80,
			wantErr:     true,
		},
		{
			name:        "invalid port",
			authority:   "example.com:http",
			defaultPort: 80,
			wantErr:     true,
		},
		{
			name:        "bare ipv6 rejected",
			authority:   "::1",
			defaultPort: 443,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := splitAuthority(tt.authority, tt.defaultPort)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitAuthority(%q, %d) = %q, %d; want error", tt.authority, tt.defaultPort, host, port)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitAuthority(%q, %d) error = %v", tt.authority, tt.defaultPort, err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Fatalf("splitAuthority(%q, %d) = %q, %d; want %q, %d", tt.authority, tt.defaultPort, host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```powershell
go test ./internal/proxy -run TestSplitAuthority -v
```

Expected:

```text
undefined: splitAuthority
FAIL
```

- [ ] **Step 3: Implement authority parsing**

Create `internal/proxy/upstream.go`:

```go
package proxy

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func splitAuthority(authority string, defaultPort int) (string, int, error) {
	if authority == "" {
		return "", 0, errors.New("empty authority")
	}
	if defaultPort < 1 || defaultPort > 65535 {
		return "", 0, fmt.Errorf("invalid default port %d", defaultPort)
	}

	host, portString, err := net.SplitHostPort(authority)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port") {
			return "", 0, fmt.Errorf("split authority %q: %w", authority, err)
		}
		host = authority
		if strings.HasPrefix(authority, "[") || strings.HasSuffix(authority, "]") {
			if !strings.HasPrefix(authority, "[") || !strings.HasSuffix(authority, "]") {
				return "", 0, fmt.Errorf("split authority %q: %w", authority, err)
			}
			host = strings.TrimSuffix(strings.TrimPrefix(authority, "["), "]")
		}
		if host == "" {
			return "", 0, errors.New("empty host")
		}
		return host, defaultPort, nil
	}
	if host == "" {
		return "", 0, errors.New("empty host")
	}

	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port %q", portString)
	}
	return host, port, nil
}
```

- [ ] **Step 4: Run test and verify it passes**

Run:

```powershell
go test ./internal/proxy -run TestSplitAuthority -v
```

Expected:

```text
=== RUN   TestSplitAuthority
--- PASS: TestSplitAuthority
PASS
```

- [ ] **Step 5: Commit**

Run:

```powershell
gofmt -s -w internal/proxy/upstream.go internal/proxy/upstream_test.go
git add internal/proxy/upstream.go internal/proxy/upstream_test.go
git commit -m "feat: add proxy authority parsing"
```

---

### Task 3: Add constant-time proxy authentication

**Files:**
- Create: `internal/proxy/auth_test.go`
- Create: `internal/proxy/auth.go`

**Interfaces:**
- Produces: `func newAuthenticator(user, pass string) *authenticator`.
- Produces: `func (a *authenticator) authorized(req *http.Request) bool`.
- Used by: forward proxy and CONNECT tunnel handlers.

- [ ] **Step 1: Write failing tests**

Create `internal/proxy/auth_test.go`:

```go
package proxy

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatorAuthorized(t *testing.T) {
	auth := newAuthenticator("buffrelay1", "s3cret")
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{
			name:   "valid credentials",
			header: basicHeader("buffrelay1", "s3cret"),
			want:   true,
		},
		{
			name:   "wrong user",
			header: basicHeader("other", "s3cret"),
			want:   false,
		},
		{
			name:   "wrong password",
			header: basicHeader("buffrelay1", "wrong"),
			want:   false,
		},
		{
			name:   "missing header",
			header: "",
			want:   false,
		},
		{
			name:   "wrong scheme",
			header: "Bearer token",
			want:   false,
		},
		{
			name:   "broken base64",
			header: "Basic !!!broken!!!",
			want:   false,
		},
		{
			name:   "payload without colon",
			header: "Basic " + base64.StdEncoding.EncodeToString([]byte("buffrelay1")),
			want:   false,
		},
		{
			name:   "scheme case insensitive",
			header: "basic " + base64.StdEncoding.EncodeToString([]byte("buffrelay1:s3cret")),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Proxy-Authorization", tt.header)
			}
			if got := auth.authorized(req); got != tt.want {
				t.Fatalf("authorized() = %v; want %v", got, tt.want)
			}
		})
	}
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```powershell
go test ./internal/proxy -run TestAuthenticatorAuthorized -v
```

Expected:

```text
undefined: newAuthenticator
FAIL
```

- [ ] **Step 3: Implement auth**

Create `internal/proxy/auth.go`:

```go
package proxy

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

type authenticator struct {
	user []byte
	pass []byte
}

func newAuthenticator(user, pass string) *authenticator {
	return &authenticator{
		user: []byte(user),
		pass: []byte(pass),
	}
}

func (a *authenticator) authorized(req *http.Request) bool {
	header := req.Header.Get("Proxy-Authorization")
	scheme, encoded, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}

	user, pass, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return false
	}

	userOK := subtle.ConstantTimeCompare([]byte(user), a.user)
	passOK := subtle.ConstantTimeCompare([]byte(pass), a.pass)
	return userOK&passOK == 1
}
```

- [ ] **Step 4: Run auth tests**

Run:

```powershell
go test ./internal/proxy -run TestAuthenticatorAuthorized -v
```

Expected:

```text
=== RUN   TestAuthenticatorAuthorized
--- PASS: TestAuthenticatorAuthorized
PASS
```

- [ ] **Step 5: Commit**

Run:

```powershell
gofmt -s -w internal/proxy/auth.go internal/proxy/auth_test.go
git add internal/proxy/auth.go internal/proxy/auth_test.go
git commit -m "feat: add proxy basic authentication"
```

---

### Task 4: Add public nginx-disguised HTTP surface

**Files:**
- Create: `internal/proxy/server_test.go`
- Create: `internal/proxy/server.go`

**Interfaces:**
- Produces: `func New(logger *slog.Logger, user, pass string) *Server`.
- Produces: `type Server struct` implementing `http.Handler`.
- Produces public behavior: `GET /`, `GET /health`, nginx 404.

- [ ] **Step 1: Write failing public-surface tests**

Create `internal/proxy/server_test.go`:

```go
package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicSurface(t *testing.T) {
	srv := newTestServer()
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "landing page",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "Welcome to nginx!",
		},
		{
			name:       "health",
			method:     http.MethodGet,
			path:       "/health",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "unknown path",
			method:     http.MethodGet,
			path:       "/missing",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 Not Found",
		},
		{
			name:       "post landing is not public",
			method:     http.MethodPost,
			path:       "/",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 Not Found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d; want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q; want contains %q", rec.Body.String(), tt.wantBody)
			}
			if got := rec.Header().Get("Server"); got != fakeServer {
				t.Fatalf("Server header = %q; want %q", got, fakeServer)
			}
		})
	}
}

func newTestServer() *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(logger, "u", "p")
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```powershell
go test ./internal/proxy -run TestPublicSurface -v
```

Expected:

```text
undefined: Server
undefined: fakeServer
undefined: New
FAIL
```

- [ ] **Step 3: Implement the public surface**

Create `internal/proxy/server.go`:

```go
package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const fakeServer = "nginx/1.24.0"

const landingHTML = "<!DOCTYPE html>\n<html>\n<head><title>Welcome to nginx!</title></head>\n<body>\n<center><h1>Welcome to nginx!</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"
const notFoundHTML = "<html>\n<head><title>404 Not Found</title></head>\n<body>\n<center><h1>404 Not Found</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"

type Server struct {
	logger            *slog.Logger
	auth              *authenticator
	client            *http.Client
	dialer            *net.Dialer
	dialTimeout       time.Duration
	tunnelIdleTimeout time.Duration
}

func New(logger *slog.Logger, user, pass string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		Proxy:                 nil,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Server{
		logger:            logger,
		auth:              newAuthenticator(user, pass),
		client:            client,
		dialer:            dialer,
		dialTimeout:       5 * time.Second,
		tunnelIdleTimeout: 30 * time.Second,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		s.writeHTML(w, http.StatusOK, landingHTML)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		s.writePlain(w, http.StatusOK, "ok")
		return
	}
	s.writeHTML(w, http.StatusNotFound, notFoundHTML)
}

func (s *Server) writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write html failed", "err", err)
	}
}

func (s *Server) writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write plain failed", "err", err)
	}
}
```

- [ ] **Step 4: Run public tests**

Run:

```powershell
go test ./internal/proxy -run TestPublicSurface -v
```

Expected:

```text
=== RUN   TestPublicSurface
--- PASS: TestPublicSurface
PASS
```

- [ ] **Step 5: Commit**

Run:

```powershell
gofmt -s -w internal/proxy/server.go internal/proxy/server_test.go
git add internal/proxy/server.go internal/proxy/server_test.go
git commit -m "feat: add disguised public HTTP surface"
```

---

### Task 5: Add absolute-form HTTP forward proxy

**Files:**
- Modify: `internal/proxy/server_test.go`
- Modify: `internal/proxy/server.go`

**Interfaces:**
- Consumes: `(*authenticator).authorized(*http.Request) bool`.
- Consumes: `splitAuthority(authority string, defaultPort int) (string, int, error)`.
- Produces: absolute-form `http://host/path` forwarding with auth, header stripping, and 502 on upstream error.

- [ ] **Step 1: Append failing forward tests**

Append this to `internal/proxy/server_test.go`:

```go
func TestForwardRequiresAuth(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusProxyAuthRequired)
	}
	if got := rec.Header().Get("Proxy-Authenticate"); got != `Basic realm="auth"` {
		t.Fatalf("Proxy-Authenticate = %q; want Basic realm", got)
	}
}

func TestForwardProxiesRequest(t *testing.T) {
	var gotHost string
	var gotProxyAuth string
	var gotProxyConnection string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotProxyAuth = r.Header.Get("Proxy-Authorization")
		gotProxyConnection = r.Header.Get("Proxy-Connection")
		w.Header().Set("X-Upstream", "yes")
		w.Header().Set("Server", "leaky-upstream")
		w.WriteHeader(http.StatusCreated)
		if _, err := io.WriteString(w, "upstream ok"); err != nil {
			t.Fatalf("write upstream response: %v", err)
		}
	}))
	defer upstream.Close()

	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, upstream.URL+"/path?q=1", nil)
	req.Header.Set("Proxy-Authorization", basicHeader("u", "p"))
	req.Header.Set("Proxy-Connection", "keep-alive")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusCreated)
	}
	if body := rec.Body.String(); body != "upstream ok" {
		t.Fatalf("body = %q; want upstream ok", body)
	}
	if got := rec.Header().Get("X-Upstream"); got != "yes" {
		t.Fatalf("X-Upstream = %q; want yes", got)
	}
	if got := rec.Header().Get("Server"); got != fakeServer {
		t.Fatalf("Server header = %q; want %q", got, fakeServer)
	}
	if gotProxyAuth != "" {
		t.Fatalf("Proxy-Authorization leaked upstream: %q", gotProxyAuth)
	}
	if gotProxyConnection != "" {
		t.Fatalf("Proxy-Connection leaked upstream: %q", gotProxyConnection)
	}
	if gotHost == "" {
		t.Fatal("upstream saw empty Host")
	}
}

func TestForwardUpstreamError(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	req.Header.Set("Proxy-Authorization", basicHeader("u", "p"))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Header().Get("Server"); got != fakeServer {
		t.Fatalf("Server header = %q; want %q", got, fakeServer)
	}
}
```

- [ ] **Step 2: Run forward tests and verify they fail**

Run:

```powershell
go test ./internal/proxy -run TestForward -v
```

Expected:

```text
--- FAIL: TestForwardRequiresAuth
status = 404; want 407
FAIL
```

- [ ] **Step 3: Replace `internal/proxy/server.go` with forward-capable implementation**

Replace `internal/proxy/server.go`:

```go
package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const fakeServer = "nginx/1.24.0"

const landingHTML = "<!DOCTYPE html>\n<html>\n<head><title>Welcome to nginx!</title></head>\n<body>\n<center><h1>Welcome to nginx!</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"
const notFoundHTML = "<html>\n<head><title>404 Not Found</title></head>\n<body>\n<center><h1>404 Not Found</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"

type Server struct {
	logger            *slog.Logger
	auth              *authenticator
	client            *http.Client
	dialer            *net.Dialer
	dialTimeout       time.Duration
	tunnelIdleTimeout time.Duration
}

func New(logger *slog.Logger, user, pass string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		Proxy:                 nil,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Server{
		logger:            logger,
		auth:              newAuthenticator(user, pass),
		client:            client,
		dialer:            dialer,
		dialTimeout:       5 * time.Second,
		tunnelIdleTimeout: 30 * time.Second,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.IsAbs() && r.URL.Host != "" {
		s.handleForward(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		s.writeHTML(w, http.StatusOK, landingHTML)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		s.writePlain(w, http.StatusOK, "ok")
		return
	}
	s.writeHTML(w, http.StatusNotFound, notFoundHTML)
}

func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	if !s.auth.authorized(r) {
		s.writeProxyAuthRequired(w)
		return
	}
	if r.URL.Scheme != "http" {
		s.writePlain(w, http.StatusBadRequest, "use CONNECT for HTTPS")
		return
	}

	host, port, err := splitAuthority(r.URL.Host, 80)
	if err != nil {
		s.logger.Debug("invalid forward authority", "authority", r.URL.Host, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}

	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	outURL := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL, r.Body)
	if err != nil {
		s.logger.Debug("build forward request failed", "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	outReq.Header = r.Header.Clone()
	stripHopHeaders(outReq.Header)
	outReq.Host = r.URL.Host

	resp, err := s.client.Do(outReq)
	if err != nil {
		s.logger.Debug("forward upstream failed", "host", host, "port", port, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Server", fakeServer)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.logger.Debug("copy forward response failed", "err", err)
	}
}

func (s *Server) writeProxyAuthRequired(w http.ResponseWriter) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Proxy-Authenticate", `Basic realm="auth"`)
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusProxyAuthRequired)
	if _, err := io.WriteString(w, "authentication required"); err != nil {
		s.logger.Debug("write auth challenge failed", "err", err)
	}
}

func (s *Server) writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write html failed", "err", err)
	}
}

func (s *Server) writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write plain failed", "err", err)
	}
}

func stripHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) || strings.EqualFold(key, "Server") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run forward and public tests**

Run:

```powershell
go test ./internal/proxy -run "Test(PublicSurface|Forward)" -v
```

Expected:

```text
--- PASS: TestPublicSurface
--- PASS: TestForwardRequiresAuth
--- PASS: TestForwardProxiesRequest
--- PASS: TestForwardUpstreamError
PASS
```

- [ ] **Step 5: Commit**

Run:

```powershell
gofmt -s -w internal/proxy/server.go internal/proxy/server_test.go
git add internal/proxy/server.go internal/proxy/server_test.go
git commit -m "feat: add authenticated HTTP forwarding"
```

---

### Task 6: Add CONNECT tunnel support

**Files:**
- Modify: `internal/proxy/server_test.go`
- Modify: `internal/proxy/server.go`

**Interfaces:**
- Consumes: `splitAuthority(authority string, defaultPort int) (string, int, error)`.
- Consumes: `(*authenticator).authorized(*http.Request) bool`.
- Produces: `CONNECT host:port` tunnel via `http.ResponseController.Hijack`.
- Produces: tunnel idle timeout through `copyIdle(dst io.Writer, src net.Conn, idle time.Duration) (int64, error)`.

- [ ] **Step 1: Append failing CONNECT tests**

Append to `internal/proxy/server_test.go`:

```go
func TestConnectRequiresAuth(t *testing.T) {
	ts := httptest.NewServer(newTestServer())
	defer ts.Close()

	conn := dialTestServer(t, ts)
	defer conn.Close()
	if _, err := io.WriteString(conn, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	line := readStatusLine(t, conn)
	if !strings.Contains(line, "407") {
		t.Fatalf("status line = %q; want 407", line)
	}
}

func TestConnectTunnelEcho(t *testing.T) {
	echo := newEchoServer(t)
	defer echo.Close()

	ts := httptest.NewServer(newTestServer())
	defer ts.Close()

	conn := dialTestServer(t, ts)
	defer conn.Close()
	target := echo.Addr().String()
	request := "CONNECT " + target + " HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Proxy-Authorization: " + basicHeader("u", "p") + "\r\n" +
		"\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	line := readStatusLine(t, conn)
	if !strings.Contains(line, "200") {
		t.Fatalf("status line = %q; want 200", line)
	}
	drainHeaders(t, conn)

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatalf("write tunnel payload: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q; want ping", string(buf))
	}
}

func dialTestServer(t *testing.T, ts *httptest.Server) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	return conn
}

func readStatusLine(t *testing.T, conn net.Conn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	return line
}

func drainHeaders(t *testing.T, conn net.Conn) {
	t.Helper()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("drain headers: %v", err)
		}
		if line == "\r\n" {
			return
		}
	}
}

type echoServer struct {
	listener net.Listener
}

func newEchoServer(t *testing.T) *echoServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo server: %v", err)
	}
	es := &echoServer{listener: listener}
	go es.serve()
	return es
}

func (e *echoServer) Addr() net.Addr {
	return e.listener.Addr()
}

func (e *echoServer) Close() error {
	return e.listener.Close()
}

func (e *echoServer) serve() {
	for {
		conn, err := e.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			if _, err := io.Copy(conn, conn); err != nil {
				return
			}
		}()
	}
}
```

Add these imports to `internal/proxy/server_test.go`:

```go
import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run CONNECT tests and verify they fail**

Run:

```powershell
go test ./internal/proxy -run TestConnect -v
```

Expected:

```text
--- FAIL: TestConnectRequiresAuth
status line = "HTTP/1.1 404 Not Found\r\n"; want 407
FAIL
```

- [ ] **Step 3: Replace `internal/proxy/server.go` with CONNECT-capable implementation**

Replace `internal/proxy/server.go`:

```go
package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const fakeServer = "nginx/1.24.0"

const landingHTML = "<!DOCTYPE html>\n<html>\n<head><title>Welcome to nginx!</title></head>\n<body>\n<center><h1>Welcome to nginx!</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"
const notFoundHTML = "<html>\n<head><title>404 Not Found</title></head>\n<body>\n<center><h1>404 Not Found</h1></center>\n<hr><center>nginx/1.24.0</center>\n</body>\n</html>\n"

type Server struct {
	logger            *slog.Logger
	auth              *authenticator
	client            *http.Client
	dialer            *net.Dialer
	dialTimeout       time.Duration
	tunnelIdleTimeout time.Duration
}

func New(logger *slog.Logger, user, pass string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		Proxy:                 nil,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Server{
		logger:            logger,
		auth:              newAuthenticator(user, pass),
		client:            client,
		dialer:            dialer,
		dialTimeout:       5 * time.Second,
		tunnelIdleTimeout: 30 * time.Second,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	if r.URL.IsAbs() && r.URL.Host != "" {
		s.handleForward(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		s.writeHTML(w, http.StatusOK, landingHTML)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		s.writePlain(w, http.StatusOK, "ok")
		return
	}
	s.writeHTML(w, http.StatusNotFound, notFoundHTML)
}

func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	if !s.auth.authorized(r) {
		s.writeProxyAuthRequired(w)
		return
	}
	if r.URL.Scheme != "http" {
		s.writePlain(w, http.StatusBadRequest, "use CONNECT for HTTPS")
		return
	}

	host, port, err := splitAuthority(r.URL.Host, 80)
	if err != nil {
		s.logger.Debug("invalid forward authority", "authority", r.URL.Host, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}

	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	outURL := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL, r.Body)
	if err != nil {
		s.logger.Debug("build forward request failed", "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	outReq.Header = r.Header.Clone()
	stripHopHeaders(outReq.Header)
	outReq.Host = r.URL.Host

	resp, err := s.client.Do(outReq)
	if err != nil {
		s.logger.Debug("forward upstream failed", "host", host, "port", port, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Server", fakeServer)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.logger.Debug("copy forward response failed", "err", err)
	}
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if !s.auth.authorized(r) {
		s.writeProxyAuthRequired(w)
		return
	}

	target := r.RequestURI
	if target == "" {
		target = r.Host
	}
	host, port, err := splitAuthority(target, 443)
	if err != nil {
		s.logger.Debug("invalid connect authority", "authority", target, "err", err)
		s.writePlain(w, http.StatusBadRequest, "bad request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.dialTimeout)
	defer cancel()
	upstream, err := s.dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		s.logger.Debug("connect upstream failed", "host", host, "port", port, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	defer upstream.Close()

	controller := http.NewResponseController(w)
	clientConn, buffered, err := controller.Hijack()
	if err != nil {
		s.logger.Debug("hijack failed", "err", err)
		return
	}
	defer clientConn.Close()

	started := time.Now()
	if _, err := io.WriteString(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		s.logger.Debug("write connect response failed", "err", err)
		return
	}
	if buffered != nil && buffered.Reader.Buffered() > 0 {
		if _, err := io.CopyN(upstream, buffered, int64(buffered.Reader.Buffered())); err != nil {
			s.logger.Debug("copy buffered connect bytes failed", "err", err)
			return
		}
	}

	done := make(chan struct{}, 2)
	go s.copyTunnel("client_to_upstream", upstream, clientConn, done)
	go s.copyTunnel("upstream_to_client", clientConn, upstream, done)
	<-done
	_ = clientConn.Close()
	_ = upstream.Close()
	<-done
	s.logger.Info("tunnel closed", "target", target, "duration", time.Since(started).String())
}

func (s *Server) copyTunnel(direction string, dst io.Writer, src net.Conn, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	bytesCopied, err := copyIdle(dst, src, s.tunnelIdleTimeout)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		s.logger.Debug("tunnel copy stopped", "direction", direction, "bytes", bytesCopied, "err", err)
		return
	}
	s.logger.Debug("tunnel copy closed", "direction", direction, "bytes", bytesCopied)
}

func copyIdle(dst io.Writer, src net.Conn, idle time.Duration) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := src.SetReadDeadline(time.Now().Add(idle)); err != nil {
			return total, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func (s *Server) writeProxyAuthRequired(w http.ResponseWriter) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Proxy-Authenticate", `Basic realm="auth"`)
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusProxyAuthRequired)
	if _, err := io.WriteString(w, "authentication required"); err != nil {
		s.logger.Debug("write auth challenge failed", "err", err)
	}
}

func (s *Server) writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write html failed", "err", err)
	}
}

func (s *Server) writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Server", fakeServer)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		s.logger.Debug("write plain failed", "err", err)
	}
}

func stripHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) || strings.EqualFold(key, "Server") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run CONNECT tests**

Run:

```powershell
go test ./internal/proxy -run TestConnect -v
```

Expected:

```text
--- PASS: TestConnectRequiresAuth
--- PASS: TestConnectTunnelEcho
PASS
```

- [ ] **Step 5: Run all proxy tests with race detection**

Run:

```powershell
go test -race ./internal/proxy -v
```

Expected:

```text
PASS
ok   github.com/zumlabs/zums-proxy-relay/internal/proxy
```

- [ ] **Step 6: Commit**

Run:

```powershell
gofmt -s -w internal/proxy/server.go internal/proxy/server_test.go
git add internal/proxy/server.go internal/proxy/server_test.go
git commit -m "feat: add authenticated CONNECT tunneling"
```

---

### Task 7: Add process config, slog, and graceful shutdown

**Files:**
- Create: `cmd/server/main_test.go`
- Create: `cmd/server/main.go`

**Interfaces:**
- Produces: `func loadConfig(getenv func(string) string) (config, error)`.
- Produces: `func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error`.
- Consumes: `proxy.New(logger, user, pass)`.

- [ ] **Step 1: Write failing config tests**

Create `cmd/server/main_test.go`:

```go
package main

import "testing"

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    config
		wantErr bool
	}{
		{
			name: "valid explicit config",
			env: map[string]string{
				"HOST":      "127.0.0.1",
				"PORT":      "9000",
				"AUTH_USER": "u",
				"AUTH_PASS": "p",
				"DEBUG":     "true",
			},
			want: config{
				host:     "127.0.0.1",
				port:     9000,
				authUser: "u",
				authPass: "p",
				debug:    true,
			},
		},
		{
			name: "valid defaults",
			env: map[string]string{
				"AUTH_USER": "u",
				"AUTH_PASS": "p",
			},
			want: config{
				host:     "0.0.0.0",
				port:     8080,
				authUser: "u",
				authPass: "p",
				debug:    false,
			},
		},
		{
			name: "missing user",
			env: map[string]string{
				"AUTH_PASS": "p",
			},
			wantErr: true,
		},
		{
			name: "missing pass",
			env: map[string]string{
				"AUTH_USER": "u",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			env: map[string]string{
				"PORT":      "nope",
				"AUTH_USER": "u",
				"AUTH_PASS": "p",
			},
			wantErr: true,
		},
		{
			name: "port zero",
			env: map[string]string{
				"PORT":      "0",
				"AUTH_USER": "u",
				"AUTH_PASS": "p",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadConfig(func(key string) string {
				return tt.env[key]
			})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("loadConfig() = %+v; want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("loadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run config tests and verify they fail**

Run:

```powershell
go test ./cmd/server -run TestLoadConfig -v
```

Expected:

```text
undefined: config
undefined: loadConfig
FAIL
```

- [ ] **Step 3: Implement `cmd/server/main.go`**

Create `cmd/server/main.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/zumlabs/zums-proxy-relay/internal/proxy"
)

type config struct {
	host     string
	port     int
	authUser string
	authPass string
	debug    bool
}

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: level}))
	addr := net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
	server := &http.Server{
		Addr:              addr,
		Handler:           proxy.New(logger, cfg.authUser, cfg.authPass),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("relay listening", "addr", addr, "auth", "required")
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen and serve: %w", err)
	}
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		host:     valueOr(getenv("HOST"), "0.0.0.0"),
		authUser: getenv("AUTH_USER"),
		authPass: getenv("AUTH_PASS"),
		debug:    getenv("DEBUG") == "true",
	}
	portString := valueOr(getenv("PORT"), "8080")
	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return config{}, fmt.Errorf("invalid port %q", portString)
	}
	cfg.port = port
	if cfg.authUser == "" || cfg.authPass == "" {
		return config{}, errors.New("auth_user and auth_pass are required; refusing to start an open proxy")
	}
	return cfg, nil
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
```

- [ ] **Step 4: Run config tests**

Run:

```powershell
go test ./cmd/server -run TestLoadConfig -v
```

Expected:

```text
=== RUN   TestLoadConfig
--- PASS: TestLoadConfig
PASS
```

- [ ] **Step 5: Run all tests with race detection**

Run:

```powershell
go test -race ./...
```

Expected:

```text
ok   github.com/zumlabs/zums-proxy-relay/cmd/server
ok   github.com/zumlabs/zums-proxy-relay/internal/proxy
```

- [ ] **Step 6: Build binary**

Run:

```powershell
go build ./...
```

Expected: no output and exit code 0.

- [ ] **Step 7: Commit**

Run:

```powershell
gofmt -s -w cmd/server/main.go cmd/server/main_test.go
git add cmd/server/main.go cmd/server/main_test.go
git commit -m "feat: add service entrypoint and graceful shutdown"
```

---

### Task 8: Add project tooling, Dockerfile, license, env example, and CI

**Files:**
- Create: `Makefile`
- Create: `.golangci.yml`
- Modify: `.env.example`
- Create: `Dockerfile`
- Create: `LICENSE`
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Produces: local commands `make build`, `make test`, `make fmt`, `make vet`, `make lint`.
- Produces: GitHub Actions workflow `CI`.
- Produces: portable container image build.

- [ ] **Step 1: Create `Makefile`**

Write `Makefile`:

```makefile
BINARY := bin/zums-proxy-relay

.PHONY: build run test fmt vet lint

build:
	go build -trimpath -o $(BINARY) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	golangci-lint run
```

- [ ] **Step 2: Create `.golangci.yml`**

Write `.golangci.yml`:

```yaml
version: "2"
linters:
  default: standard
```

- [ ] **Step 3: Update `.env.example`**

Write `.env.example`:

```dotenv
# zums-proxy-relay environment variables

# Railway injects PORT automatically. Local fallback is 8080.
PORT=8080

# Listen address.
HOST=0.0.0.0

# Required. Empty values make the server exit to avoid an open proxy.
AUTH_USER=
AUTH_PASS=

# true enables debug-level slog output.
DEBUG=false
```

- [ ] **Step 4: Create `Dockerfile`**

Write `Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zums-proxy-relay ./cmd/server

FROM scratch
COPY --from=build /out/zums-proxy-relay /zums-proxy-relay
EXPOSE 8080
ENTRYPOINT ["/zums-proxy-relay"]
```

- [ ] **Step 5: Create `LICENSE`**

Write `LICENSE`:

```text
MIT License

Copyright (c) 2026 zumlabs

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 6: Create GitHub Actions workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable
          cache: true

      - name: Check formatting
        run: test -z "$(gofmt -l .)"

      - name: Vet
        run: go vet ./...

      - name: Build
        run: go build ./...

      - name: Test with race detector
        run: go test -race ./...

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable
          cache: true

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
```

- [ ] **Step 7: Verify local checks**

Run:

```powershell
gofmt -s -w .
go vet ./...
go test -race ./...
go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 8: Commit**

Run:

```powershell
git add Makefile .golangci.yml .env.example Dockerfile LICENSE .github/workflows/ci.yml
git commit -m "chore: add tooling and CI"
```

---

### Task 9: Rewrite README for portfolio and Railway

**Files:**
- Modify: `README.md`

**Interfaces:**
- Produces: public-facing README that explains what the project does, how to run it, how to test it, and how to deploy it on Railway.

- [ ] **Step 1: Replace `README.md`**

Write `README.md`:

```markdown
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
{"level":"INFO","msg":"relay listening","addr":"0.0.0.0:8080","auth":"required"}
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
curl -x http://wrong:wrong@localhost:8080 https://example.com -o /dev/null -w "%{http_code}\n"
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
```

- [ ] **Step 2: Verify README references existing files**

Run:

```powershell
go test -race ./...
go build ./...
```

Expected: both commands exit 0.

- [ ] **Step 3: Commit**

Run:

```powershell
git add README.md
git commit -m "docs: rewrite README for Go relay"
```

---

### Task 10: Final verification and GitHub push

**Files:**
- No source edits expected.
- Remote: `https://github.com/zumlabs/zums-proxy-relay.git`

**Interfaces:**
- Produces: pushed `main` branch on GitHub.
- Produces: CI run visible in GitHub Actions.

- [ ] **Step 1: Inspect repo state**

Run:

```powershell
git status --short
git log --oneline -10
```

Expected: only intended files are tracked, and no accidental secrets are staged.

- [ ] **Step 2: Run full local verification**

Run:

```powershell
gofmt -s -w .
go vet ./...
go test -race ./...
go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 3: Run local smoke test**

In terminal 1:

```powershell
$env:AUTH_USER="u"
$env:AUTH_PASS="p"
go run ./cmd/server
```

In terminal 2:

```powershell
curl.exe http://localhost:8080/
curl.exe http://localhost:8080/health
curl.exe -x http://u:p@localhost:8080 http://example.com -o NUL -w "%{http_code}\n"
curl.exe -x http://u:p@localhost:8080 https://example.com -o NUL -w "%{http_code}\n"
curl.exe -x http://wrong:wrong@localhost:8080 https://example.com -o NUL -w "%{http_code}\n"
```

Expected:

```text
GET / contains Welcome to nginx!
GET /health returns ok
HTTP forward returns 200
CONNECT returns 200
wrong auth returns 407
```

- [ ] **Step 4: Confirm no secrets are committed**

Run:

```powershell
git grep -n "buffrelaypass\|AUTH_PASS=.*[^=]\|relay-pass\|s3cret" -- . ":!docs/superpowers/**"
```

Expected: no real secret values. Test-only strings like `s3cret` must only appear in tests or docs.

- [ ] **Step 5: Add remote**

Run:

```powershell
git remote add origin https://github.com/zumlabs/zums-proxy-relay.git
```

If the remote already exists, run:

```powershell
git remote set-url origin https://github.com/zumlabs/zums-proxy-relay.git
```

- [ ] **Step 6: Push**

Run:

```powershell
git push -u origin main
```

Expected:

```text
branch 'main' set up to track 'origin/main'
```

- [ ] **Step 7: Check CI**

Open GitHub Actions for `zumlabs/zums-proxy-relay` and confirm workflow `CI` is green. If using CLI:

```powershell
gh run list --repo zumlabs/zums-proxy-relay --limit 5
```

Expected: latest `CI` run is `completed` and `success`.

---

## Self-review checklist

- Spec coverage: Go rewrite, `/health`, nginx disguise, required env auth, constant-time auth, timeouts, graceful shutdown, slog, tests, CI, Dockerfile, Railway README, MIT license are all covered.
- Placeholder scan: no `TBD`, no `TODO`, no vague “implement later” steps.
- Type consistency: `newAuthenticator`, `authorized`, `splitAuthority`, `New`, `Server`, and `loadConfig` signatures are consistent across tasks.
- Scope check: SOCKS5, multi-user auth, CD pipeline, metrics, and rate limiting are excluded as intentional YAGNI.
