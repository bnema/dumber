package webkit

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubHitTest simulates a WebKit HitTestResult for testing.
type stubHitTest struct {
	linkURI     string
	imageURI    string
	isLink      bool
	isImage     bool
	isEditable  bool
	isSelection bool
}

func TestBuildMenuContextFromHitTest(t *testing.T) {
	t.Run("image hit test populates ImageURI", func(t *testing.T) {
		hit := stubHitTest{
			imageURI: "https://example.com/photo.png",
			isImage:  true,
		}
		ctx := buildMenuContextFromStubHitTest(hit, true, false, 100, 200)

		assert.Equal(t, "https://example.com/photo.png", ctx.ImageURI)
		assert.Equal(t, 100, ctx.X)
		assert.Equal(t, 200, ctx.Y)
		assert.True(t, ctx.CanGoBack)
		assert.False(t, ctx.CanGoForward)
	})

	t.Run("link hit test populates LinkURI", func(t *testing.T) {
		hit := stubHitTest{
			linkURI: "https://example.com/page",
			isLink:  true,
		}
		ctx := buildMenuContextFromStubHitTest(hit, false, true, 50, 75)

		assert.Equal(t, "https://example.com/page", ctx.LinkURI)
		assert.Empty(t, ctx.ImageURI)
		assert.False(t, ctx.CanGoBack)
		assert.True(t, ctx.CanGoForward)
	})

	t.Run("editable and selection flags propagate", func(t *testing.T) {
		hit := stubHitTest{
			isEditable:  true,
			isSelection: true,
		}
		ctx := buildMenuContextFromStubHitTest(hit, false, false, 0, 0)

		assert.True(t, ctx.IsEditable)
		assert.True(t, ctx.HasSelection)
	})

	t.Run("combined image and link", func(t *testing.T) {
		hit := stubHitTest{
			linkURI:  "https://example.com/link",
			imageURI: "https://example.com/img.jpg",
			isLink:   true,
			isImage:  true,
		}
		ctx := buildMenuContextFromStubHitTest(hit, true, true, 10, 20)

		assert.Equal(t, "https://example.com/link", ctx.LinkURI)
		assert.Equal(t, "https://example.com/img.jpg", ctx.ImageURI)
		assert.True(t, ctx.CanGoBack)
		assert.True(t, ctx.CanGoForward)
	})

	t.Run("empty hit test produces empty context", func(t *testing.T) {
		hit := stubHitTest{}
		ctx := buildMenuContextFromStubHitTest(hit, false, false, 0, 0)

		assert.Empty(t, ctx.LinkURI)
		assert.Empty(t, ctx.ImageURI)
		assert.False(t, ctx.HasSelection)
		assert.False(t, ctx.IsEditable)
	})
}

func TestResolveImageData(t *testing.T) {
	t.Run("rejects empty image URI", func(t *testing.T) {
		resolver := &contextMenuResolver{}
		_, err := resolver.ResolveImageData(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty image URI")
	})

	t.Run("rejects unsupported URI scheme", func(t *testing.T) {
		resolver := &contextMenuResolver{}
		_, err := resolver.ResolveImageData(context.Background(), "file:///tmp/image.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported URI scheme")
	})

	t.Run("decodes data URI images", func(t *testing.T) {
		imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
		uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)

		resolver := &contextMenuResolver{}
		data, err := resolver.ResolveImageData(context.Background(), uri)
		require.NoError(t, err)
		assert.Equal(t, imageBytes, data.Bytes)
		assert.Equal(t, "image/png", data.MimeType)
	})

	t.Run("fetches image bytes from HTTP", func(t *testing.T) {
		imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg; charset=utf-8")
			w.Write(imageBytes)
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{allowPrivateHosts: true}
		data, err := resolver.ResolveImageData(context.Background(), srv.URL+"/image.png")
		require.NoError(t, err)
		assert.Equal(t, imageBytes, data.Bytes)
		assert.Equal(t, "image/png", data.MimeType)
	})

	t.Run("rejects DNS lookup errors", func(t *testing.T) {
		resolver := &contextMenuResolver{
			lookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
				return nil, errors.New("dns failed")
			},
		}

		_, err := resolver.ResolveImageData(context.Background(), "https://image.example/photo.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dns failed")
	})

	t.Run("falls back to sniffed MIME type when header is generic", func(t *testing.T) {
		imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(imageBytes)
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{allowPrivateHosts: true}
		data, err := resolver.ResolveImageData(context.Background(), srv.URL+"/image.png")
		require.NoError(t, err)
		assert.Equal(t, imageBytes, data.Bytes)
		assert.Equal(t, "image/png", data.MimeType)
	})

	t.Run("rejects DNS-resolved private hosts", func(t *testing.T) {
		resolver := &contextMenuResolver{
			lookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			},
		}

		_, err := resolver.ResolveImageData(context.Background(), "https://image.example/photo.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private host")
	})

	t.Run("connect-time dial skips private addresses and uses public ones", func(t *testing.T) {
		var dialed []string
		resolver := &contextMenuResolver{
			lookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{
					{IP: net.ParseIP("127.0.0.1")},
					{IP: net.ParseIP("93.184.216.34")},
				}, nil
			},
			dialContext: func(_ context.Context, _ string, addr string) (net.Conn, error) {
				dialed = append(dialed, addr)
				conn, peer := net.Pipe()
				t.Cleanup(func() {
					_ = conn.Close()
					_ = peer.Close()
				})
				return conn, nil
			},
		}

		conn, err := resolver.dialImageHostContext(context.Background(), "tcp", "image.example:443")
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, []string{"93.184.216.34:443"}, dialed)
	})

	t.Run("rejects private literal addresses at connect time", func(t *testing.T) {
		resolver := &contextMenuResolver{}

		_, err := resolver.dialImageHostContext(context.Background(), "tcp", "127.0.0.1:443")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private host")
	})

	t.Run("redirect check rejects private targets", func(t *testing.T) {
		resolver := &contextMenuResolver{
			lookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			},
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://redirect.example/photo.png", http.NoBody)
		require.NoError(t, err)
		err = resolver.imageFetchClient(context.Background()).CheckRedirect(req, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private host")
	})

	t.Run("rejects non-image HTTP content", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body>not an image</body></html>"))
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{allowPrivateHosts: true}
		_, err := resolver.ResolveImageData(context.Background(), srv.URL+"/page.html")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an image")
	})

	t.Run("rejects spoofed image headers when sniffed bytes are not an image", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("<html><body>not really an image</body></html>"))
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{allowPrivateHosts: true}
		_, err := resolver.ResolveImageData(context.Background(), srv.URL+"/spoofed.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an image")
		assert.Contains(t, err.Error(), "text/html")
		assert.Contains(t, err.Error(), "image/png")
		assert.Contains(t, err.Error(), "/spoofed.png")
	})

	t.Run("returns error on HTTP failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{allowPrivateHosts: true}
		_, err := resolver.ResolveImageData(context.Background(), srv.URL+"/missing.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("rejects oversized bodies", func(t *testing.T) {
		resolver := &contextMenuResolver{
			allowPrivateHosts: true,
			client: &http.Client{
				Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"image/png"}},
						Body:       io.NopCloser(newSizedByteReader(maxImageFetchBytes + 1)),
					}, nil
				}),
			},
		}

		_, err := resolver.ResolveImageData(context.Background(), "https://example.com/image.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too large")
	})

	t.Run("returns error on invalid URL", func(t *testing.T) {
		resolver := &contextMenuResolver{}
		_, err := resolver.ResolveImageData(context.Background(), "://bad-url")
		require.Error(t, err)
	})
}

func TestWebkitMenuDelegator_OpenLinkNewTab(t *testing.T) {
	var gotURI string
	wv := &WebView{
		OnLinkMiddleClick: func(uri string) bool {
			gotURI = uri
			return true
		},
	}
	delegator := &webkitMenuDelegator{wv: wv}

	err := delegator.DelegateMenuAction(context.Background(), port.MenuActionOpenLinkNewTab, port.MenuContext{LinkURI: "https://example.com/new"})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/new", gotURI)
}

func TestWebkitMenuDelegator_OpenLinkNewTabRequiresHandlerSuccess(t *testing.T) {
	wv := &WebView{
		OnLinkMiddleClick: func(string) bool {
			return false
		},
	}
	delegator := &webkitMenuDelegator{wv: wv}

	err := delegator.DelegateMenuAction(context.Background(), port.MenuActionOpenLinkNewTab, port.MenuContext{LinkURI: "https://example.com/new"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action not handled")
}

// buildMenuContextFromStubHitTest is the test-only equivalent of buildMenuContextFromHitTest
// that works with our stub type instead of requiring real WebKit objects.
func buildMenuContextFromStubHitTest(hit stubHitTest, canGoBack, canGoForward bool, x, y int) port.MenuContext {
	ctx := port.MenuContext{
		CanGoBack:    canGoBack,
		CanGoForward: canGoForward,
		X:            x,
		Y:            y,
	}

	if hit.isLink {
		ctx.LinkURI = hit.linkURI
	}
	if hit.isImage {
		ctx.ImageURI = hit.imageURI
	}
	ctx.HasSelection = hit.isSelection
	ctx.IsEditable = hit.isEditable

	return ctx
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type sizedByteReader struct {
	remaining int
}

func newSizedByteReader(size int) *sizedByteReader {
	return &sizedByteReader{remaining: size}
}

func (r *sizedByteReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}
