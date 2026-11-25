package favicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
)

func homeTestPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Set(0, 0, color.RGBA{R: 1, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>`

func TestHomeIconDiscoverRanking(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><html><head>
<link rel="shortcut icon" href="/small.png" sizes="16x16 32x32">
<link rel="icon" href="/plain.ico">
<link rel="apple-touch-icon" href="touch.png" sizes="120x120">
<link rel="apple-touch-icon" href="/touch180.png" sizes="180x180">
<link rel="Manifest" href="/m/site.webmanifest">
<link rel="icon" type="image/svg+xml" href="/logo.svg">
</head><body><link rel="icon" href="/ignored-body.png"></body></html>`)
	})
	mux.HandleFunc("/m/site.webmanifest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"icons":[{"src":"i192.png","sizes":"192x192"},{"src":"i512.png","sizes":"512x512"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := NewHomeIconSource().DiscoverIconURLs(context.Background(), srv.URL+"/app/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/logo.svg", "/m/i512.png", "/m/i192.png", "/touch180.png", "/app/touch.png",
		"/small.png", "/plain.ico", "/favicon.ico",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != srv.URL+w {
			t.Fatalf("candidate %d = %s, want %s (all: %v)", i, got[i], srv.URL+w, got)
		}
	}
}

func TestHomeIconDiscoverPageFailureStillTriesFaviconICO(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	got, err := NewHomeIconSource().DiscoverIconURLs(context.Background(), srv.URL+"/x")
	if err == nil {
		t.Fatal("expected page error")
	}
	if len(got) != 1 || got[0] != srv.URL+"/favicon.ico" {
		t.Fatalf("got %v", got)
	}
}

func TestHomeIconDiscoverUsesFinalURLAndBase(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dir/page", http.StatusFound)
	})
	mux.HandleFunc("/dir/page", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<head><link rel="icon" href="a.png" sizes="64x64"></head>`)
	})
	mux.HandleFunc("/based", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<head><base href="/assets/"><link rel="icon" href="b.png" sizes="64x64"></head>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	src := NewHomeIconSource()
	got, err := src.DiscoverIconURLs(context.Background(), srv.URL+"/start")
	if err != nil || got[0] != srv.URL+"/dir/a.png" {
		t.Fatalf("redirect base: %v %v", got, err)
	}
	got, err = src.DiscoverIconURLs(context.Background(), srv.URL+"/based")
	if err != nil || got[0] != srv.URL+"/assets/b.png" {
		t.Fatalf("base href: %v %v", got, err)
	}
}

func TestHomeIconRedirectCap(t *testing.T) {
	mux := http.NewServeMux()
	// /hop/N redirects to /hop/N-1; /hop/0 serves an icon.
	mux.HandleFunc("/hop/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/hop/"), "%d", &n)
		if n == 0 {
			_, _ = w.Write(homeTestPNG(t, 4))
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	src := NewHomeIconSource()
	if _, err := src.FetchIcon(context.Background(), srv.URL, srv.URL+"/hop/3"); err != nil {
		t.Fatalf("3 redirects should pass: %v", err)
	}
	if _, err := src.FetchIcon(context.Background(), srv.URL, srv.URL+"/hop/4"); err == nil {
		t.Fatal("4 redirects should fail")
	}
}

func TestHomeIconFetchSizeCapAndSniff(t *testing.T) {
	big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, homeIconMaxBytes)...)
	mux := http.NewServeMux()
	mux.HandleFunc("/big.png", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(big) })
	mux.HandleFunc("/logo.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, testSVG)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "<html></html>") })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	src := NewHomeIconSource()
	if _, err := src.FetchIcon(context.Background(), srv.URL, srv.URL+"/big.png"); !errors.Is(err, appport.ErrHomeIconNotFound) {
		t.Fatalf("oversized: %v", err)
	}
	img, err := src.FetchIcon(context.Background(), srv.URL, srv.URL+"/logo.svg")
	if err != nil || img.ContentType != "image/svg+xml" {
		t.Fatalf("svg sniff: %v %+v", err, img)
	}
	if _, err := src.FetchIcon(context.Background(), srv.URL, srv.URL+"/html"); err == nil {
		t.Fatal("html must not be an icon")
	}
}

func TestHomeIconDiscoverTruncatesHTML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<head><link rel="icon" href="/early.png" sizes="48x48">`)
		_, _ = w.Write(bytes.Repeat([]byte(" "), homeIconMaxHTML))
		_, _ = fmt.Fprint(w, `<link rel="icon" type="image/svg+xml" href="/late.svg"></head>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got, err := NewHomeIconSource().DiscoverIconURLs(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range got {
		if strings.HasSuffix(u, "/late.svg") {
			t.Fatalf("link past the cap was parsed: %v", got)
		}
	}
	if got[0] != srv.URL+"/early.png" {
		t.Fatalf("got %v", got)
	}
}

func TestParseManifestIcons(t *testing.T) {
	base := mustParseURL(t, "http://jelly.local:8096/web/manifest.json")
	got := parseManifestIcons([]byte(`{"icons":[
{"src":"a.png","sizes":"48x48 96x96"},
{"src":"/abs.svg","type":"image/svg+xml"},
{"src":"javascript:alert(1)","sizes":"512x512"},
{"src":"","sizes":"512x512"}]}`), base)
	ranked := rankCandidates(got)
	want := []string{"http://jelly.local:8096/abs.svg", "http://jelly.local:8096/web/a.png"}
	if len(ranked) != len(want) || ranked[0] != want[0] || ranked[1] != want[1] {
		t.Fatalf("got %v", ranked)
	}
	if parseManifestIcons([]byte("not json"), base) != nil {
		t.Fatal("bad json must give nothing")
	}
}

func TestLargestSize(t *testing.T) {
	cases := map[string]int{"": 0, "16x16": 16, "16x16 64X64 32x32": 64, "any": 1 << 16, "bogus 10x": 0}
	for in, want := range cases {
		if got := largestSize(in); got != want {
			t.Errorf("largestSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestHomeIconReadLocal(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	src := NewHomeIconSource()
	img, err := src.ReadLocalIcon(context.Background(), write("a.svg", []byte(testSVG)))
	if err != nil || img.ContentType != "image/svg+xml" {
		t.Fatalf("svg: %v %+v", err, img)
	}
	img, err = src.ReadLocalIcon(context.Background(), write("a.png", homeTestPNG(t, 8)))
	if err != nil || img.ContentType != "image/png" {
		t.Fatalf("png: %v %+v", err, img)
	}
	big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, homeIconMaxBytes)...)
	if _, err := src.ReadLocalIcon(context.Background(), write("big.png", big)); !errors.Is(err, appport.ErrHomeIconNotFound) {
		t.Fatalf("big: %v", err)
	}
	if _, err := src.ReadLocalIcon(context.Background(), write("x.txt", []byte("hello"))); err == nil {
		t.Fatal("text must fail")
	}
}

func TestHomeIconSVGConvertsTo128(t *testing.T) {
	out, err := NewImageConverter().Convert(context.Background(), []byte(testSVG), "image/svg+xml", []int{128})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out.SizedPNG[128]))
	if err != nil || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("bad 128 png: %v %v", err, img.Bounds())
	}
}

func TestHomeIconCacheRoundTripAndKey(t *testing.T) {
	c := NewHomeIconCache(filepath.Join(t.TempDir(), "home"))
	ctx := context.Background()
	if _, _, ok := c.Get(ctx, "http://10.0.0.2:8096/"); ok {
		t.Fatal("unexpected hit")
	}
	before := time.Now().Add(-time.Second)
	if err := c.Put(ctx, "http://10.0.0.2:8096/", []byte("png")); err != nil {
		t.Fatal(err)
	}
	data, at, ok := c.Get(ctx, "HTTP://10.0.0.2:8096/")
	if !ok || string(data) != "png" || at.Before(before) {
		t.Fatalf("get: %q %v %v", data, at, ok)
	}
	if _, _, ok := c.Get(ctx, "http://10.0.0.2:3000/"); ok {
		t.Fatal("different port must not collide")
	}
}

// Two loopback servers on different ports: the "service" and an "other LAN
// host". Loopback is private, so only the service's own host:port is allowed.
func newCrossOriginServers(t *testing.T) (service, other *httptest.Server) {
	t.Helper()
	otherMux := http.NewServeMux()
	otherMux.HandleFunc("/icon.png", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(homeTestPNG(t, 8)) })
	otherMux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"icons":[{"src":"/icon.png","sizes":"512x512"}]}`)
	})
	other = httptest.NewServer(otherMux)
	t.Cleanup(other.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `<head><link rel="manifest" href="%s/manifest.json"><link rel="icon" href="/own.png" sizes="32x32"></head>`, other.URL)
	})
	mux.HandleFunc("/own.png", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(homeTestPNG(t, 8)) })
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/icon.png", http.StatusFound)
	})
	service = httptest.NewServer(mux)
	t.Cleanup(service.Close)
	return service, other
}

func TestHomeIconPrivateTargetsLimitedToServiceHost(t *testing.T) {
	service, other := newCrossOriginServers(t)
	src := NewHomeIconSource()
	ctx := context.Background()

	if _, err := src.FetchIcon(ctx, service.URL, service.URL+"/own.png"); err != nil {
		t.Fatalf("service's own host must be allowed: %v", err)
	}
	if _, err := src.FetchIcon(ctx, service.URL, other.URL+"/icon.png"); err == nil {
		t.Fatal("icon link to another private host:port must be rejected")
	}
	if _, err := src.FetchIcon(ctx, service.URL, service.URL+"/redirect"); err == nil {
		t.Fatal("redirect to another private host:port must be rejected")
	}
	got, err := src.DiscoverIconURLs(ctx, service.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range got {
		if strings.HasPrefix(u, other.URL) {
			t.Fatalf("manifest on another private host must not be read: %v", got)
		}
	}
}

func TestHomeIconPublicTargetsAllowed(t *testing.T) {
	service, other := newCrossOriginServers(t)
	src := NewHomeIconSource()
	// Treat loopback as public to stand in for internet hosts.
	src.isPrivate = func(netip.Addr) bool { return false }
	ctx := context.Background()

	if _, err := src.FetchIcon(ctx, service.URL, other.URL+"/icon.png"); err != nil {
		t.Fatalf("public icon host must be allowed: %v", err)
	}
	if _, err := src.FetchIcon(ctx, service.URL, service.URL+"/redirect"); err != nil {
		t.Fatalf("redirect to a public host must be allowed: %v", err)
	}
	got, err := src.DiscoverIconURLs(ctx, service.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != other.URL+"/icon.png" {
		t.Fatalf("public manifest icon should rank first: %v", got)
	}
}

// http.ProxyFromEnvironment caches the environment once per process, so the
// test inspects the transport instead of setting HTTP_PROXY.
func TestHomeIconSourceBypassesProxy(t *testing.T) {
	transport, ok := NewHomeIconSource().client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", NewHomeIconSource().client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("home icon client must not use a proxy: the dial check would see the proxy, not the target")
	}
	if transport.DialContext == nil || !transport.DisableKeepAlives {
		t.Fatal("home icon client must dial through the address check for every request")
	}
}
