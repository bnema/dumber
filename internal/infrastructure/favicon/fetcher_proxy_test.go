package favicon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func proxyTestFetcher(resolved map[string][]net.IP) *Fetcher {
	f := NewFetcher()
	f.envProxy = func(*http.Request) (*url.URL, error) { return url.Parse("http://proxy.example:3128") }
	f.lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		if ips, ok := resolved[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
	return f
}

func proxyRequest(t *testing.T, ctx context.Context, raw string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestFetcherProxyRefusesPrivateTargets(t *testing.T) {
	f := proxyTestFetcher(map[string][]net.IP{
		"public.example": {net.ParseIP("93.184.216.34")},
		"lan.example":    {net.ParseIP("192.168.1.10")},
		"cgnat.example":  {net.ParseIP("100.64.1.1")},
		"mixed.example":  {net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.1")},
		"10.0.0.5":       {net.ParseIP("10.0.0.5")},
		"nat64.example":  {net.ParseIP("64:ff9b::a00:1")},
	})
	ctx := context.Background()
	cases := map[string]bool{
		"https://public.example/favicon.ico": true,
		"https://lan.example/favicon.ico":    false,
		"https://cgnat.example/favicon.ico":  false,
		"https://mixed.example/favicon.ico":  false,
		"http://10.0.0.5/favicon.ico":        false,
		"https://nat64.example/favicon.ico":  false,
		"http://localhost/favicon.ico":       false,
		"https://unresolvable.example/x.ico": false,
	}
	for raw, wantAllowed := range cases {
		proxyURL, err := f.proxyFunc(proxyRequest(t, ctx, raw))
		if wantAllowed && (err != nil || proxyURL == nil) {
			t.Errorf("%s: want proxied, got %v %v", raw, proxyURL, err)
		}
		if !wantAllowed && err == nil {
			t.Errorf("%s: want refused, got proxy %v", raw, proxyURL)
		}
	}
}

func TestFetcherProxyAllowsSameOriginLocalPage(t *testing.T) {
	f := proxyTestFetcher(nil)
	ctx := context.WithValue(context.Background(), allowedLocalOriginKey, allowedLocalOrigin("http://localhost:8080/app"))
	if _, err := f.proxyFunc(proxyRequest(t, ctx, "http://localhost:8080/favicon.ico")); err != nil {
		t.Fatalf("same-origin localhost icon must keep working: %v", err)
	}
	if _, err := f.proxyFunc(proxyRequest(t, ctx, "http://localhost:9090/favicon.ico")); err == nil {
		t.Fatal("other localhost origin must be refused")
	}
}

func TestFetcherProxyNoEnvProxySkipsResolution(t *testing.T) {
	f := NewFetcher()
	f.envProxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	f.lookupIP = func(context.Context, string) ([]net.IP, error) {
		t.Fatal("must not resolve when no proxy is configured")
		return nil, nil
	}
	proxyURL, err := f.proxyFunc(proxyRequest(t, context.Background(), "https://lan.example/x.ico"))
	if err != nil || proxyURL != nil {
		t.Fatalf("got %v %v", proxyURL, err)
	}
}

func TestFetcherTransportsUseProxyFunc(t *testing.T) {
	for name, f := range map[string]*Fetcher{
		"NewFetcher":           NewFetcher(),
		"NewFetcherWithClient": NewFetcherWithClient(&http.Client{}, ""),
	} {
		tr, ok := f.client.Transport.(*http.Transport)
		if !ok || tr.Proxy == nil {
			t.Fatalf("%s: transport must use the guarded proxy func", name)
		}
	}
}
