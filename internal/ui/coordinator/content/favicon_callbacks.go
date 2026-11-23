package content

import (
	"context"
	"strings"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/puregotk/v4/gdk"
)

func (c *Coordinator) setupFaviconCallbacks(
	ctx context.Context,
	paneID entity.PaneID,
	wv port.WebView,
	callbacks *port.WebViewCallbacks,
) {
	faviconGen := wv.Generation()
	callbacks.OnFaviconChanged = func(favicon port.Texture) {
		if wv.Generation() != faviconGen {
			return
		}
		if gdkTexture, ok := favicon.(*gdk.Texture); ok {
			c.onFaviconChanged(ctx, paneID, wv, gdkTexture)
		}
	}
	callbacks.OnFaviconURLChanged = func(pageURL string, iconURLs []string) {
		if wv.Generation() != faviconGen || c.faviconAdapter == nil || pageURL == "" || len(iconURLs) == 0 {
			return
		}
		c.faviconAdapter.RefreshFromIconURLs(ctx, pageURL, iconURLs, func(texture *gdk.Texture) {
			if texture == nil || wv.Generation() != faviconGen {
				return
			}
			currentWV := c.getWebViewLocked(paneID)
			if currentWV != wv {
				return
			}
			if strings.TrimSpace(currentWV.URI()) != strings.TrimSpace(pageURL) {
				return
			}
			c.updateStackedFaviconForPane(ctx, paneID, texture)
		})
	}
}
