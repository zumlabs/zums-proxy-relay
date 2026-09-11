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

	// ponytail: no userinfo ("user@host:80") or bare-IPv6-with-port recovery; reject until a caller needs them.
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return "", 0, fmt.Errorf("invalid authority %q", authority)
		}
		host := authority[1:end]
		if host == "" {
			return "", 0, errors.New("empty host")
		}
		rest := authority[end+1:]
		switch {
		case rest == "":
			return host, defaultPort, nil
		case rest[0] == ':':
			port, err := parsePort(rest[1:])
			if err != nil {
				return "", 0, err
			}
			return host, port, nil
		default:
			return "", 0, fmt.Errorf("invalid authority %q", authority)
		}
	}

	if !strings.Contains(authority, ":") {
		return authority, defaultPort, nil
	}

	host, portString, err := net.SplitHostPort(authority)
	if err != nil {
		return "", 0, fmt.Errorf("split authority %q: %w", authority, err)
	}
	if host == "" {
		return "", 0, errors.New("empty host")
	}
	port, err := parsePort(portString)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return port, nil
}
