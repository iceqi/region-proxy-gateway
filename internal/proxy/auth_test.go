package proxy

import "testing"

func TestParseBasicProxyAuthorization(t *testing.T) {
	credentials, ok := ParseBasicProxyAuthorization("Basic cHJveHk6c2VjcmV0")
	if !ok {
		t.Fatal("expected header to parse")
	}

	if credentials.Username != "proxy" {
		t.Fatalf("username = %q, want %q", credentials.Username, "proxy")
	}
	if credentials.Password != "secret" {
		t.Fatalf("password = %q, want %q", credentials.Password, "secret")
	}
}

func TestParseBasicProxyAuthorizationRejectsNonBasicScheme(t *testing.T) {
	_, ok := ParseBasicProxyAuthorization("Bearer token")
	if ok {
		t.Fatal("expected non-basic header to be rejected")
	}
}

func TestParseBasicProxyAuthorizationAcceptsCaseInsensitiveBasicScheme(t *testing.T) {
	credentials, ok := ParseBasicProxyAuthorization("basic cHJveHk6c2VjcmV0")
	if !ok {
		t.Fatal("expected lowercase basic scheme to parse")
	}

	if credentials.Username != "proxy" {
		t.Fatalf("username = %q, want %q", credentials.Username, "proxy")
	}
	if credentials.Password != "secret" {
		t.Fatalf("password = %q, want %q", credentials.Password, "secret")
	}
}

func TestParseBasicProxyAuthorizationRejectsInvalidCredentials(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "invalid base64",
			header: "Basic !!!",
		},
		{
			name:   "missing password separator",
			header: "Basic bm9wYXNzd29yZHNlcGFyYXRvcg==",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, ok := ParseBasicProxyAuthorization(test.header)
			if ok {
				t.Fatal("expected header to be rejected")
			}
		})
	}
}

func TestCheckPassword(t *testing.T) {
	if !CheckPassword("secret", "secret") {
		t.Fatal("expected matching password to pass")
	}

	if CheckPassword("wrong", "secret") {
		t.Fatal("expected wrong password to fail")
	}

	if CheckPassword("", "") {
		t.Fatal("expected empty expected password to fail")
	}
}
