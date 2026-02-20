package cli

import "testing"

func TestNormalizeWToken(t *testing.T) {
	jwt := "abc.def.ghi"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain jwt",
			input: jwt,
			want:  jwt,
		},
		{
			name:  "bearer prefix",
			input: "Bearer " + jwt,
			want:  jwt,
		},
		{
			name:  "json access token",
			input: `{"accessToken":"abc.def.ghi","expirationTime":1771540095000}`,
			want:  jwt,
		},
		{
			name:  "url encoded json",
			input: `%7B%22accessToken%22%3A%22abc.def.ghi%22%2C%22expirationTime%22%3A1771540095000%7D`,
			want:  jwt,
		},
		{
			name:  "chrome copied partially encoded payload",
			input: `{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`,
			want:  jwt,
		},
		{
			name:  "query string payload",
			input: `accessToken=abc.def.ghi&expirationTime=1771540095000`,
			want:  jwt,
		},
		{
			name:  "cookie key value payload",
			input: `__wtoken=abc.def.ghi`,
			want:  jwt,
		},
		{
			name:  "quoted escaped json payload",
			input: `"{\"accessToken\":\"abc.def.ghi\",\"expirationTime\":1771540095000}"`,
			want:  jwt,
		},
		{
			name:  "opaque token fallback",
			input: "opaque-token-value",
			want:  "opaque-token-value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeWToken(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeWToken(%q): want %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestExtractWTokenFromCookieInputs(t *testing.T) {
	jwt := "abc.def.ghi"

	tests := []struct {
		name    string
		cookies []string
		want    string
	}{
		{
			name:    "simple __wtoken cookie",
			cookies: []string{"__wtoken=abc.def.ghi"},
			want:    jwt,
		},
		{
			name:    "cookie header value",
			cookies: []string{"foo=1; __wtoken=abc.def.ghi; bar=2"},
			want:    jwt,
		},
		{
			name:    "encoded __wtoken payload",
			cookies: []string{"__wtoken={%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}"},
			want:    jwt,
		},
		{
			name:    "access token cookie alias",
			cookies: []string{"foo=1; accessToken=abc.def.ghi"},
			want:    jwt,
		},
		{
			name:    "missing token",
			cookies: []string{"foo=1; bar=2"},
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractWTokenFromCookieInputs(tc.cookies)
			if got != tc.want {
				t.Fatalf("extractWTokenFromCookieInputs(%v): want %q, got %q", tc.cookies, tc.want, got)
			}
		})
	}
}

func TestExtractRefreshTokenFromCookieInputs(t *testing.T) {
	refresh := "refresh_token_123"

	tests := []struct {
		name    string
		cookies []string
		want    string
	}{
		{
			name:    "simple __wrtoken cookie",
			cookies: []string{"__wrtoken=refresh_token_123"},
			want:    refresh,
		},
		{
			name:    "cookie header value",
			cookies: []string{"foo=1; __wrtoken=refresh_token_123; bar=2"},
			want:    refresh,
		},
		{
			name:    "json refresh token payload",
			cookies: []string{`session=1; state={"refreshToken":"refresh_token_123"}`},
			want:    refresh,
		},
		{
			name:    "refresh token cookie alias",
			cookies: []string{"foo=1; refresh_token=refresh_token_123"},
			want:    refresh,
		},
		{
			name:    "missing refresh token",
			cookies: []string{"foo=1; bar=2"},
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractRefreshTokenFromCookieInputs(tc.cookies)
			if got != tc.want {
				t.Fatalf("extractRefreshTokenFromCookieInputs(%v): want %q, got %q", tc.cookies, tc.want, got)
			}
		})
	}
}

func TestBuildAuthContextNormalizesWTokenVariants(t *testing.T) {
	jwt := "abc.def.ghi"

	auth := buildAuthContext(globalFlags{
		WToken: `{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`,
	})
	if auth.WToken != jwt {
		t.Fatalf("expected normalized token %q, got %q", jwt, auth.WToken)
	}

	auth = buildAuthContext(globalFlags{
		Cookies: []string{`foo=1; __wtoken={%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`},
	})
	if auth.WToken != jwt {
		t.Fatalf("expected token extracted from cookie payload %q, got %q", jwt, auth.WToken)
	}

	auth = buildAuthContext(globalFlags{
		WToken: `{%22accessToken%22:%22abc.def.ghi%22%2C%22refreshToken%22:%22refresh_token_123%22}`,
	})
	if auth.RefreshToken != "refresh_token_123" {
		t.Fatalf("expected refresh token extracted from payload, got %q", auth.RefreshToken)
	}

	auth = buildAuthContext(globalFlags{
		Cookies: []string{`foo=1; __wrtoken=refresh_token_123`},
	})
	if auth.RefreshToken != "refresh_token_123" {
		t.Fatalf("expected refresh token extracted from cookie, got %q", auth.RefreshToken)
	}

	auth = buildAuthContext(globalFlags{
		WRefreshToken: "%22refresh_token_456%22",
	})
	if auth.RefreshToken != "refresh_token_456" {
		t.Fatalf("expected normalized refresh token refresh_token_456, got %q", auth.RefreshToken)
	}
}
