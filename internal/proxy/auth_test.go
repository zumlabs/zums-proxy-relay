package proxy

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatorAuthorized(t *testing.T) {
	auth := newAuthenticator("relayuser", "s3cret")
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{
			name:   "valid credentials",
			header: basicHeader("relayuser", "s3cret"),
			want:   true,
		},
		{
			name:   "wrong user",
			header: basicHeader("other", "s3cret"),
			want:   false,
		},
		{
			name:   "wrong password",
			header: basicHeader("relayuser", "wrong"),
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
			header: "Basic " + base64.StdEncoding.EncodeToString([]byte("relayuser")),
			want:   false,
		},
		{
			name:   "scheme case insensitive",
			header: "basic " + base64.StdEncoding.EncodeToString([]byte("relayuser:s3cret")),
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
