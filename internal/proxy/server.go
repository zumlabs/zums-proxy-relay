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
