package homehttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/port"
)

func TestDoSendsHeadersAndReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-Api-Key") != "k" || r.UserAgent() != "dumber-home" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(r.Cookies()) != 0 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "s", Value: "1"})
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New()
	for range 2 { // second call proves no cookie jar
		resp, err := c.Do(context.Background(), port.HomeHTTPRequest{
			Method: "post", URL: srv.URL, Headers: map[string]string{"X-Api-Key": "k"}, Body: []byte("x"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusTeapot || string(resp.Body) != `{"ok":true}` {
			t.Fatalf("resp = %d %q", resp.StatusCode, resp.Body)
		}
	}
}

func TestDoSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, MaxResponseBytes+1))
	}))
	defer srv.Close()
	_, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}

	exact := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, MaxResponseBytes))
	}))
	defer exact.Close()
	if _, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: exact.URL}); err != nil {
		t.Fatalf("exact cap: %v", err)
	}
}

func TestDoTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL, Timeout: 50 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout not applied")
	}
}

func TestDoRedirectCap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n int
		_, _ = fmt.Sscan(r.URL.Query().Get("n"), &n)
		if r.URL.Query().Get("stop") == strconv.Itoa(n) {
			_, _ = w.Write([]byte("done"))
			return
		}
		http.Redirect(w, r, srv.URL+"/?n="+strconv.Itoa(n+1)+"&stop="+r.URL.Query().Get("stop"), http.StatusFound)
	}))
	defer srv.Close()

	resp, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL + "/?n=0&stop=3"})
	if err != nil || string(resp.Body) != "done" {
		t.Fatalf("3 redirects: resp=%q err=%v", resp.Body, err)
	}
	_, err = New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL + "/?n=0&stop=4"})
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("4 redirects: err = %v, want ErrTooManyRedirects", err)
	}
}

func TestDoRejectsSchemes(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com", "dumb://home", "http://"} {
		if _, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: raw}); !errors.Is(err, ErrUnsupportedScheme) {
			t.Errorf("%s: err = %v, want ErrUnsupportedScheme", raw, err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL}); !errors.Is(err, ErrUnsupportedScheme) {
		t.Fatalf("redirect to file: err = %v, want ErrUnsupportedScheme", err)
	}
}

func TestDoErrorsDoNotLeakSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close() // connection refused

	_, err := New().Do(context.Background(), port.HomeHTTPRequest{
		URL:     addr + "/api/pathsecret?apikey=querysecret",
		Headers: map[string]string{"Authorization": "Bearer headersecret"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); strings.Contains(msg, "headersecret") || strings.Contains(msg, "querysecret") ||
		strings.Contains(msg, "pathsecret") {
		t.Fatalf("error leaks a secret: %q", msg)
	}

	_, err = New().Do(context.Background(), port.HomeHTTPRequest{
		URL:     addr,
		Headers: map[string]string{"X-Bad": "bad\nheadersecret"},
	})
	if err == nil || strings.Contains(err.Error(), "headersecret") {
		t.Fatalf("invalid header error = %v", err)
	}
}

func TestDoRejectsCrossOriginRedirectWithSecrets(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		leaked.Store(true)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	_, err := New().Do(context.Background(), port.HomeHTTPRequest{
		Method: http.MethodPost, URL: srv.URL, Headers: map[string]string{"X-Api-Key": "SECRET"}, Body: []byte("body"),
	})
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Fatalf("err = %v, want ErrCrossOriginRedirect", err)
	}
	if leaked.Load() {
		t.Fatal("second server received the redirected request")
	}
}

func TestDoRejectsHTTPSDowngradeRedirect(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same host and port as the TLS server, plain scheme.
		http.Redirect(w, r, "http://"+r.Host+"/", http.StatusFound)
	}))
	defer srv.Close()

	c := New()
	c.http.Transport = srv.Client().Transport
	if _, err := c.Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL}); !errors.Is(err, ErrCrossOriginRedirect) {
		t.Fatalf("err = %v, want ErrCrossOriginRedirect", err)
	}
}

func TestDoFollowsSameOriginRedirectWithHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte(r.Header.Get("X-Api-Key")))
			return
		}
		http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	resp, err := New().Do(context.Background(), port.HomeHTTPRequest{URL: srv.URL, Headers: map[string]string{"X-Api-Key": "k"}})
	if err != nil || string(resp.Body) != "k" {
		t.Fatalf("resp=%q err=%v", resp.Body, err)
	}
}
