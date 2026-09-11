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
	var gotHost, gotProxyAuth, gotProxyConnection string
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

func newTestServer() *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(logger, "u", "p")
}
