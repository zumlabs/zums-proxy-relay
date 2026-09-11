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
