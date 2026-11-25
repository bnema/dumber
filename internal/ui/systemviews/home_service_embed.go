package systemviews

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"bnema.dev/dumber/internal/domain/entity"
)

const (
	// homeEmbedSandbox lets the embedded app run, but never top-navigate the
	// home page.
	homeEmbedSandbox = "allow-scripts allow-same-origin allow-forms allow-popups"
)

// homeEmbedView is one iframe to render.
type homeEmbedView struct {
	Name string
	URL  string
	// Height comes normalized by the layout validation (240 by default,
	// 80 to 2000).
	Height int
}

// homeEmbedURL returns raw when it is an absolute http(s) URL of a host
// other than the internal system view origin, without credentials.
func homeEmbedURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	// IDNA-mapped like the browser does, so fullwidth or ideographic-dot
	// spellings of the internal host are refused too.
	if entity.IsBlockedEmbedURL(raw) {
		return "", false
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.User = nil
	return parsed.String(), true
}

// homeEmbedFor returns the iframe of service, when it has a valid embed.
func homeEmbedFor(service entity.HomeService) (homeEmbedView, bool) {
	if service.Widget == nil || service.Widget.Type != entity.HomeServiceWidgetEmbed {
		return homeEmbedView{}, false
	}
	target, ok := homeEmbedURL(service.Widget.URL)
	if !ok {
		return homeEmbedView{Name: service.Name}, false
	}
	return homeEmbedView{Name: service.Name, URL: target, Height: service.Widget.Height}, true
}

// homeEmbedKey identifies what an embed region shows: the same key keeps
// the iframe node. Empty means an empty region.
func homeEmbedKey(service entity.HomeService) string {
	if view, ok := homeEmbedFor(service); ok {
		return view.URL + "\x00" + strconv.Itoa(view.Height)
	}
	if homeEmbedRejected(service) {
		return "\x00rejected"
	}
	return ""
}

// changedHomeEmbedRegionsLocked returns the embed regions whose key changed
// since the last push and records the new keys. Caller holds App.mu.
func (a *App) changedHomeEmbedRegionsLocked() []DOMRegion {
	var regions []DOMRegion
	seen := make(map[string]bool)
	for _, group := range a.homeData.Load.Layout.Groups {
		for i := range group.Services {
			service := &group.Services[i]
			seen[service.ID] = true
			key := homeEmbedKey(*service)
			if a.homeSvc.embeds[service.ID] == key {
				continue // a missing entry is an empty region
			}
			if a.homeSvc.embeds == nil {
				a.homeSvc.embeds = make(map[string]string)
			}
			a.homeSvc.embeds[service.ID] = key
			view, ok := homeEmbedFor(*service)
			regions = append(regions, DOMRegion{
				Name: homeServiceEmbedRegion(service.ID),
				HTML: mustRenderComponent(HomeServiceEmbed(view, ok, homeEmbedRejected(*service))),
			})
		}
	}
	for id := range a.homeSvc.embeds {
		if !seen[id] {
			delete(a.homeSvc.embeds, id)
		}
	}
	return regions
}

// homeEmbedRejected reports an embed widget whose url failed the check.
func homeEmbedRejected(service entity.HomeService) bool {
	if service.Widget == nil || service.Widget.Type != entity.HomeServiceWidgetEmbed {
		return false
	}
	_, ok := homeEmbedURL(service.Widget.URL)
	return !ok
}

// syncHomeEmbedsLocked pushes the embeds whose url or height changed. An
// unchanged embed keeps its iframe node and its loaded page. Caller holds
// App.mu.
func (a *App) syncHomeEmbedsLocked(ctx context.Context) {
	regions := a.changedHomeEmbedRegionsLocked()
	if len(regions) == 0 {
		return
	}
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return
	}
	if err := updater.UpdateRegions(RegionUpdate{Regions: regions}); err != nil {
		logActionMountError(ctx, err, nil)
	}
}
