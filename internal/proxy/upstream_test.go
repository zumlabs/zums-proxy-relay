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
		{
			name:        "bare ipv6 with port rejected",
			authority:   "::1:8080",
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
