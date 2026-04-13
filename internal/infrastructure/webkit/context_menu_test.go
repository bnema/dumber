package webkit

import (
	"context"
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

	t.Run("fetches image bytes from HTTP", func(t *testing.T) {
		imageBytes := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.Write(imageBytes)
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{}
		data, err := resolver.ResolveImageData(context.Background(), srv.URL+"/image.png")
		require.NoError(t, err)
		assert.Equal(t, imageBytes, data.Bytes)
	})

	t.Run("returns error on HTTP failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		resolver := &contextMenuResolver{}
		_, err := resolver.ResolveImageData(context.Background(), srv.URL+"/missing.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("returns error on invalid URL", func(t *testing.T) {
		resolver := &contextMenuResolver{}
		_, err := resolver.ResolveImageData(context.Background(), "://bad-url")
		require.Error(t, err)
	})
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
