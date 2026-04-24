package cef

import (
	"net/url"
	"testing"
)

func TestToActualInternalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "page root",
			in:   "dumb://history",
			want: "https://dumber.invalid/history",
		},
		{
			name: "history page root",
			in:   "dumb://history",
			want: "https://dumber.invalid/history",
		},
		{
			name: "error page root",
			in:   "dumb://error",
			want: "https://dumber.invalid/error",
		},
		{
			name: "favorites page root",
			in:   "dumb://favorites",
			want: "https://dumber.invalid/favorites",
		},
		{
			name: "config page root",
			in:   "dumb://config",
			want: "https://dumber.invalid/config",
		},
		{
			name: "api path stays at origin root",
			in:   "dumb://history/api/message",
			want: "https://dumber.invalid/api/message",
		},
		{
			name: "config api path stays at origin root",
			in:   "dumb://config/api/config/default",
			want: "https://dumber.invalid/api/config/default",
		},
		{
			name: "root asset stays at origin root",
			in:   "dumb://history/favicon.ico",
			want: "https://dumber.invalid/favicon.ico",
		},
		{
			name: "page subroute stays under page namespace",
			in:   "dumb://history/crash?url=https%3A%2F%2Fexample.com",
			want: "https://dumber.invalid/history/crash?url=https%3A%2F%2Fexample.com",
		},
		{
			name: "actual internal URL unchanged",
			in:   "https://dumber.invalid/config",
			want: "https://dumber.invalid/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := toActualInternalURL(tt.in); got != tt.want {
				t.Fatalf("toActualInternalURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveAPIPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{
			name: "nil url returns false",
			in:   "",
			want: "",
			ok:   false,
		},
		{
			name: "conceptual api host",
			in:   "dumb://api/clipboard-set",
			want: "/api/clipboard-set",
			ok:   true,
		},
		{
			name: "conceptual history api path",
			in:   "dumb://history/api/message",
			want: "/api/message",
			ok:   true,
		},
		{
			name: "conceptual config api path",
			in:   "dumb://config/api/config/default",
			want: "/api/config/default",
			ok:   true,
		},
		{
			name: "actual internal api path",
			in:   "https://dumber.invalid/api/focus-sync",
			want: "/api/focus-sync",
			ok:   true,
		},
		{
			name: "non api path",
			in:   "https://dumber.invalid/home",
			want: "",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var parsed *url.URL
			if tt.in != "" {
				var err error
				parsed, err = url.Parse(tt.in)
				if err != nil {
					t.Fatalf("url.Parse(%q) error = %v", tt.in, err)
				}
			}
			got, ok := resolveAPIPath(parsed)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("resolveAPIPath(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestToConceptualInternalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "page root",
			in:   "https://dumber.invalid/history",
			want: "dumb://history",
		},
		{
			name: "history page root",
			in:   "https://dumber.invalid/history",
			want: "dumb://history",
		},
		{
			name: "error page root",
			in:   "https://dumber.invalid/error",
			want: "dumb://error",
		},
		{
			name: "favorites page root",
			in:   "https://dumber.invalid/favorites",
			want: "dumb://favorites",
		},
		{
			name: "config page root",
			in:   "https://dumber.invalid/config",
			want: "dumb://config",
		},
		{
			name: "page subroute",
			in:   "https://dumber.invalid/history/crash?url=https%3A%2F%2Fexample.com",
			want: "dumb://history/crash?url=https%3A%2F%2Fexample.com",
		},
		{
			name: "root asset is left unchanged",
			in:   "https://dumber.invalid/favicon.ico",
			want: "https://dumber.invalid/favicon.ico",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := toConceptualInternalURL(tt.in); got != tt.want {
				t.Fatalf("toConceptualInternalURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
