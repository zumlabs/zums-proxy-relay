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
