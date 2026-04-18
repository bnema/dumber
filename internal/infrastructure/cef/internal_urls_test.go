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
			in:   "dumb://home",
			want: "https://dumber.invalid/home",
		},
		{
			name: "api path stays at origin root",
			in:   "dumb://home/api/message",
			want: "https://dumber.invalid/api/message",
		},
		{
			name: "root asset stays at origin root",
			in:   "dumb://home/favicon.ico",
			want: "https://dumber.invalid/favicon.ico",
		},
		{
			name: "page subroute stays under page namespace",
			in:   "dumb://home/crash?url=https%3A%2F%2Fexample.com",
			want: "https://dumber.invalid/home/crash?url=https%3A%2F%2Fexample.com",
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
			name: "conceptual api host",
			in:   "dumb://api/clipboard-set",
			want: "/api/clipboard-set",
			ok:   true,
		},
		{
			name: "conceptual page api path",
			in:   "dumb://home/api/message",
			want: "/api/message",
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
			parsed, err := url.Parse(tt.in)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", tt.in, err)
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
			in:   "https://dumber.invalid/home",
			want: "dumb://home",
		},
		{
			name: "page subroute",
			in:   "https://dumber.invalid/home/crash?url=https%3A%2F%2Fexample.com",
			want: "dumb://home/crash?url=https%3A%2F%2Fexample.com",
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
