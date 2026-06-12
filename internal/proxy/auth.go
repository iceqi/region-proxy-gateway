package proxy

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

type Credentials struct {
	Username string
	Password string
}

func ParseBasicProxyAuthorization(header string) (Credentials, bool) {
	scheme, encoded, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return Credentials{}, false
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Credentials{}, false
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return Credentials{}, false
	}

	return Credentials{
		Username: username,
		Password: password,
	}, true
}

func CheckPassword(got, expected string) bool {
	if expected == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
