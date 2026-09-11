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
