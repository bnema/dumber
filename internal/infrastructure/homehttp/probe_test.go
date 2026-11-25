package homehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbe_Statuses(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"ok", http.StatusOK},
		{"unauthorized", http.StatusUnauthorized},
		{"unavailable", http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodHead, r.Method)
				assert.Equal(t, userAgent, r.Header.Get("User-Agent"))
				assert.Empty(t, r.Header.Get("Cookie"))
				w.WriteHeader(tt.code)
			}))
			defer srv.Close()
			res, err := NewProber().Probe(context.Background(), srv.URL, time.Second)
			require.NoError(t, err)
			assert.Equal(t, tt.code, res.StatusCode)
			assert.False(t, res.TLSUnverified)
		})
	}
}

func TestProbe_RedirectNotFollowed(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer srv.Close()
	res, err := NewProber().Probe(context.Background(), srv.URL+"/", time.Second)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Equal(t, []string{"/"}, paths)
}

func TestProbe_HeadRejectedRetriesGet(t *testing.T) {
	for _, code := range []int{http.StatusMethodNotAllowed, http.StatusNotImplemented} {
		var mu sync.Mutex
		var methods []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			methods = append(methods, r.Method)
			mu.Unlock()
			if r.Method == http.MethodHead {
				w.WriteHeader(code)
				return
			}
			_, _ = w.Write([]byte(strings.Repeat("x", 1<<16)))
		}))
		res, err := NewProber().Probe(context.Background(), srv.URL, time.Second)
		srv.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Equal(t, []string{http.MethodHead, http.MethodGet}, methods)
	}
}

func TestProbe_SelfSignedTLSIsUnverifiedButUp(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	res, err := NewProber().Probe(context.Background(), srv.URL, time.Second)
	require.NoError(t, err)
	assert.True(t, res.TLSUnverified)
	assert.Zero(t, res.StatusCode)
}

func TestProbe_ClosedPortFailsWithHostOnly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	_, err = NewProber().Probe(context.Background(), "http://"+addr+"/secret?token=abc", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), addr)
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "token")
}

func TestProbe_RejectsNonHTTP(t *testing.T) {
	_, err := NewProber().Probe(context.Background(), "file:///etc/passwd", time.Second)
	require.ErrorIs(t, err, ErrUnsupportedScheme)
}

func TestProbe_DropsUserinfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	u := strings.Replace(srv.URL, "http://", "http://user:pass@", 1)
	res, err := NewProber().Probe(context.Background(), u, time.Second)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)
}

func TestProbe_LatencyCoversOnlyGetAfterFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	// Each now() call advances a fake clock by one second.
	clock := time.Unix(0, 0)
	p := NewProber()
	p.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	res, err := p.Probe(context.Background(), srv.URL, time.Second)
	require.NoError(t, err)
	assert.Equal(t, time.Second, res.Latency)
}
