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
	// NewRequestWithContext infers Content-Length from the body type, not the
	// cloned header, so body-carrying forwards would be re-framed as chunked;
	// carry the original value explicitly.
	outReq.ContentLength = r.ContentLength

	resp, err := s.client.Do(outReq)
	if err != nil {
		s.logger.Debug("forward upstream failed", "host", host, "port", port, "err", err)
		s.writePlain(w, http.StatusBadGateway, "bad gateway")
		return
	}
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = upstream.Close() }()

	controller := http.NewResponseController(w)
	clientConn, buffered, err := controller.Hijack()
	if err != nil {
		s.logger.Debug("hijack failed", "err", err)
		return
	}
	defer func() { _ = clientConn.Close() }()

	started := time.Now()
	if _, err := io.WriteString(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		s.logger.Debug("write connect response failed", "err", err)
		return
	}
	// net/http may have read TLS-client-hello bytes past the CONNECT headers
	// before the hijack; flush exactly those buffered bytes before tunneling.
	if buffered != nil && buffered.Reader.Buffered() > 0 {
		if _, err := io.CopyN(upstream, buffered, int64(buffered.Reader.Buffered())); err != nil {
			s.logger.Debug("copy buffered connect bytes failed", "err", err)
			return
		}
	}

	// Join without WaitGroup: first copier finishing signals us to force-close
	// both conns, which unblocks the other copier's pending read, then we wait
	// for its signal so no goroutine outlives the handler.
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
		// Per-read deadline: each successful read restarts the idle window, so
		// active tunnels are never killed; only 30s of silence times out.
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
