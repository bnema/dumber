package favicon

import "testing"

func TestKeyCandidates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []Key
	}{
		{name: "walks public host to registrable domain", raw: "https://app.docs.example.com/path", want: []Key{"app.docs.example.com", "docs.example.com", "example.com"}},
		{name: "strips www", raw: "https://www.example.com", want: []Key{"example.com"}},
		{name: "localhost with port is exact", raw: "http://localhost:1455/path", want: []Key{"localhost:1455"}},
		{name: "ip with port is exact", raw: "http://127.0.0.1:8080/path", want: []Key{"127.0.0.1:8080"}},
		{name: "ipv6 with port is exact", raw: "http://[::1]:8080/path", want: []Key{"[::1]:8080"}},
		{name: "uses public suffix for co uk", raw: "https://news.bbc.co.uk/story", want: []Key{"news.bbc.co.uk", "bbc.co.uk"}},

		{name: "drops default https port", raw: "https://example.com:443/path", want: []Key{"example.com"}},
		{name: "non default url port is exact", raw: "https://example.com:8443/path", want: []Key{"example.com:8443"}},
		{name: "host-like string lowercases and trims trailing dot", raw: "Docs.Example.COM.", want: []Key{"docs.example.com", "example.com"}},
		{name: "strips www before preserving non default port", raw: "https://www.example.com:8443", want: []Key{"example.com:8443"}},
		{name: "host-like string with port preserves port exact", raw: "example.com:443", want: []Key{"example.com:443"}},
		{name: "unicode host normalizes to punycode", raw: "https://bücher.example/path", want: []Key{"xn--bcher-kva.example"}},
		{name: "punycode equivalent matches unicode", raw: "https://xn--bcher-kva.example/path", want: []Key{"xn--bcher-kva.example"}},
		{name: "empty input has no candidates", raw: "", want: nil},
		{name: "about url has no candidates", raw: "about:blank", want: nil},
		{name: "unsupported scheme has no candidates", raw: "file:///tmp/icon.html", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Candidates(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("Candidates(%q) len = %d (%v), want %d (%v)", tt.raw, len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Candidates(%q)[%d] = %q, want %q (all got %v)", tt.raw, i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestCanonicalKey(t *testing.T) {
	key, ok := CanonicalKey("https://www.Example.COM.:443/path")
	if !ok {
		t.Fatal("CanonicalKey returned false")
	}
	if key != "example.com" {
		t.Fatalf("CanonicalKey = %q, want example.com", key)
	}

	if key, ok := CanonicalKey("about:blank"); ok || key != "" {
		t.Fatalf("CanonicalKey(about:blank) = %q, %v; want empty false", key, ok)
	}
}
