package proxy

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
	var gotHost, gotPath, gotQuery, gotProxyAuth, gotProxyConnection string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
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
	if gotPath != "/path" {
		t.Fatalf("path = %q; want /path", gotPath)
	}
	if gotQuery != "q=1" {
		t.Fatalf("query = %q; want q=1", gotQuery)
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

func TestConnectRequiresAuth(t *testing.T) {
	ts := httptest.NewServer(newTestServer())
	defer ts.Close()

	conn := dialTestServer(t, ts)
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	reader := bufio.NewReader(conn)
	line := readStatusLine(t, reader)
	if !strings.Contains(line, "407") {
		t.Fatalf("status line = %q; want 407", line)
	}
}

func TestConnectTunnelEcho(t *testing.T) {
	echo := newEchoServer(t)
	defer func() { _ = echo.Close() }()

	ts := httptest.NewServer(newTestServer())
	defer ts.Close()

	conn := dialTestServer(t, ts)
	defer func() { _ = conn.Close() }()
	target := echo.Addr().String()
	request := "CONNECT " + target + " HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Proxy-Authorization: " + basicHeader("u", "p") + "\r\n" +
		"\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	reader := bufio.NewReader(conn)
	line := readStatusLine(t, reader)
	if !strings.Contains(line, "200") {
		t.Fatalf("status line = %q; want 200", line)
	}
	drainHeaders(t, reader)

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

func readStatusLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	return line
}

func drainHeaders(t *testing.T, reader *bufio.Reader) {
	t.Helper()
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
			defer func() { _ = conn.Close() }()
			if _, err := io.Copy(conn, conn); err != nil {
				return
			}
		}()
	}
}
