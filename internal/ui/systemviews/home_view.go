package systemviews

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	browserurl "bnema.dev/dumber/internal/domain/url"
)

// Home regions. Widgets use homeWidgetRegionPrefix + their index.
const (
	homeRegionErrors       = "home-errors"
	homeRegionHeader       = "home-header"
	homeRegionBookmarks    = "home-bookmarks"
	homeRegionGroups       = "home-groups"
	homeRegionSuggestions  = "home-suggestions"
	homeWidgetRegionPrefix = "home-widget-"
)

// homeServiceIcons enables the /api/home-icon image above the fallback glyph.
const homeServiceIcons = true

// homeDefaultSearchEngine is used when the browser sends no search engine.
const homeDefaultSearchEngine = "https://duckduckgo.com/?q=%s"

// homeSuggestionLimit caps the history suggestions under the search field.
const homeSuggestionLimit = 8

// homeRow is one link row of a list widget.
type homeRow struct {
	URL   string
	Title string
}

// homeWidgetData is a built-in widget with the data it shows.
type homeWidgetData struct {
	Widget entity.HomeBuiltinWidget
	Rows   []homeRow
	Error  string
	// Loading marks a list widget whose rows are still being fetched.
	Loading bool
	// Clock fields, computed in the page.
	Time string
	Date string
}

// homeRenderData is everything the home page renders.
type homeRenderData struct {
	Load    dto.HomeLoadResponse
	Widgets []homeWidgetData
}

func homeTitle(load dto.HomeLoadResponse) string {
	if title := strings.TrimSpace(load.Layout.Settings.Title); title != "" {
		return title
	}
	return entity.HomeDefaultTitle
}

func homeColumns(load dto.HomeLoadResponse) int {
	columns := load.Layout.Settings.Columns
	if columns < entity.HomeMinColumns || columns > entity.HomeMaxColumns {
		return entity.HomeDefaultColumns
	}
	return columns
}

func homeGridStyle(load dto.HomeLoadResponse) string {
	return "--sv-home-columns: " + strconv.Itoa(homeColumns(load)) + ";"
}

func homeWidgetLimit(widget entity.HomeBuiltinWidget) int {
	if widget.Limit < entity.HomeMinLimit || widget.Limit > entity.HomeMaxLimit {
		return entity.HomeDefaultLimit
	}
	return widget.Limit
}

func homeHasErrors(load dto.HomeLoadResponse) bool {
	return load.ParseError != nil || len(load.ValidationErrors) > 0
}

func homeParseErrorLocation(parseErr *dto.HomeParseError) string {
	if parseErr == nil {
		return ""
	}
	file := parseErr.File
	if file == "" {
		file = "home.toml"
	}
	return fmt.Sprintf("%s:%d:%d", file, parseErr.Line, parseErr.Column)
}

func homeWidgetTitle(widget entity.HomeBuiltinWidget) string {
	switch widget.Type {
	case entity.HomeWidgetFavorites:
		if tag := strings.TrimSpace(widget.Tag); tag != "" {
			return "Favorites · " + tag
		}
		return "Favorites"
	case entity.HomeWidgetTopSites:
		return "Top sites"
	case entity.HomeWidgetRecent:
		return "Recent"
	default:
		return string(widget.Type)
	}
}

// homeSearchURL resolves what the user typed in the search widget: a URL is
// opened as is, anything else goes to engine with %s replaced by the encoded
// query.
func homeSearchURL(engine, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	if browserurl.LooksLikeURL(query) {
		if target := browserurl.Normalize(query); isHomeNavigableURL(target) {
			return target
		}
	}
	engine = strings.TrimSpace(engine)
	if engine == "" || !strings.Contains(engine, "%s") {
		engine = homeDefaultSearchEngine
	}
	target := strings.Replace(engine, "%s", url.QueryEscape(query), 1)
	if !isHomeNavigableURL(target) {
		return ""
	}
	return target
}

// isHomeNavigableURL reports whether the home page may navigate itself to
// raw. Script-capable schemes (javascript:, data:, blob:, ...) would run in
// the privileged page with bridge access, so only these schemes pass.
func isHomeNavigableURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	case "dumb", "file":
		return true
	default:
		return false
	}
}

// homeClockText formats now for the clock widget.
func homeClockText(format entity.HomeClockFormat, now time.Time) (clock, date string) {
	layout := "15:04"
	if format == entity.HomeClock12h {
		layout = "3:04 PM"
	}
	return now.Format(layout), now.Format("Monday, January 2")
}

func homeFaviconURL(raw string) string {
	if browserurl.IsSystemviewPageURL(raw) {
		return ""
	}
	domain := browserurl.CanonicalDomain(raw)
	if domain == "" {
		return ""
	}
	query := url.Values{}
	query.Set("domain", domain)
	query.Set("size", "32")
	return "/api/favicon?" + query.Encode()
}

func homeRowLabel(row homeRow) string {
	if title := strings.TrimSpace(row.Title); title != "" {
		return title
	}
	return stripURLScheme(row.URL)
}

func homeInitial(label string) string {
	for _, r := range strings.TrimSpace(label) {
		return strings.ToUpper(string(r))
	}
	return "•"
}

func homeServiceIconURL(service entity.HomeService) string {
	if !homeServiceIcons || service.ID == "" {
		return ""
	}
	return "/api/home-icon?" + url.Values{"service": {service.ID}}.Encode()
}

func homeWidgetRegion(index int) string {
	return homeWidgetRegionPrefix + strconv.Itoa(index)
}

// homeRegions renders every home region, in page order.
func homeRegions(data homeRenderData) []DOMRegion {
	regions := make([]DOMRegion, 0, len(data.Widgets)+4)
	regions = append(regions,
		DOMRegion{Name: homeRegionErrors, HTML: mustRenderComponent(HomeErrors(data.Load))},
		DOMRegion{Name: homeRegionHeader, HTML: mustRenderComponent(HomeHeader(data.Load))},
	)
	for i := range data.Widgets {
		regions = append(regions, DOMRegion{Name: homeWidgetRegion(i), HTML: mustRenderComponent(HomeWidget(data.Widgets[i]))})
	}
	regions = append(regions,
		DOMRegion{Name: homeRegionBookmarks, HTML: mustRenderComponent(HomeBookmarks(data.Load.Layout.Bookmarks))},
		DOMRegion{Name: homeRegionGroups, HTML: mustRenderComponent(HomeGroups(data.Load))},
	)
	return regions
}

// changedHomeRegions returns the regions of next whose markup differs from
// previous. ok is false when the region list itself changed, which needs a
// full remount.
func changedHomeRegions(previous, next []DOMRegion) (changed []DOMRegion, ok bool) {
	if len(previous) != len(next) {
		return nil, false
	}
	for i := range next {
		if previous[i].Name != next[i].Name {
			return nil, false
		}
		if previous[i].HTML != next[i].HTML {
			changed = append(changed, next[i])
		}
	}
	return changed, true
}
