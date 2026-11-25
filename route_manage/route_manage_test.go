package route_manage

import (
	"regexp"
	"testing"
)

func TestUpstreamPath(t *testing.T) {
	tests := []struct {
		name        string
		location    string
		proxyPass   string
		regex       bool
		requestPath string
		want        string
	}{
		{
			name:        "exact route keeps proxy pass path",
			location:    "/api",
			proxyPass:   "http://127.0.0.1:9000/backend",
			requestPath: "/api",
			want:        "/backend",
		},
		{
			name:        "exact route with empty proxy pass path",
			location:    "/api",
			proxyPass:   "http://127.0.0.1:9000",
			requestPath: "/api",
			want:        "",
		},
		{
			name:        "regex route forwards request path",
			location:    "^/api/v1/.*$",
			proxyPass:   "http://127.0.0.1:9000",
			regex:       true,
			requestPath: "/api/v1/users/42",
			want:        "/api/v1/users/42",
		},
		{
			name:        "regex route prefixes proxy pass path",
			location:    "^/api/v1/.*$",
			proxyPass:   "http://127.0.0.1:9000/gateway",
			regex:       true,
			requestPath: "/api/v1/users/42",
			want:        "/gateway/api/v1/users/42",
		},
		{
			name:        "regex route ignores trailing slash when joining",
			location:    "^/api/v1/.*$",
			proxyPass:   "http://127.0.0.1:9000/gateway/",
			regex:       true,
			requestPath: "/api/v1/users/42",
			want:        "/gateway/api/v1/users/42",
		},
		{
			name:        "regex route expands numbered placeholder",
			location:    "^/api/v1/(.*)$",
			proxyPass:   "http://127.0.0.1:9000/$1",
			regex:       true,
			requestPath: "/api/v1/users/42",
			want:        "/users/42",
		},
		{
			name:        "regex route expands named placeholder",
			location:    "^/api/v1/(?P<rest>.*)$",
			proxyPass:   "http://127.0.0.1:9000/v2/${rest}",
			regex:       true,
			requestPath: "/api/v1/users/42",
			want:        "/v2/users/42",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy := Proxy{
				Location:  test.location,
				ProxyPass: test.proxyPass,
				Regex:     test.regex,
			}
			if test.regex {
				compiled, err := regexp.Compile(test.location)
				if err != nil {
					t.Fatalf("compile regex: %v", err)
				}
				proxy.regex = compiled
			}

			if got := proxy.UpstreamPath(test.requestPath); got != test.want {
				t.Fatalf("UpstreamPath(%q) = %q, want %q", test.requestPath, got, test.want)
			}
		})
	}
}
