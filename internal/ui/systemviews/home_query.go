package systemviews

import (
	"net/url"
	"sort"
	"strings"

	"bnema.dev/dumber/internal/application/dto"
	browserurl "bnema.dev/dumber/internal/domain/url"
)

// homeQueryMode is how the Home search interprets the current query.
type homeQueryMode string

const (
	homeQueryIdle        homeQueryMode = "idle"
	homeQueryFilter      homeQueryMode = "filter"
	homeQueryBang        homeQueryMode = "bang"
	homeQueryBangPartial homeQueryMode = "bang_partial"
	homeQueryBangUnknown homeQueryMode = "bang_unknown"
	homeQueryAddress     homeQueryMode = "address"
	homeQueryWeb         homeQueryMode = "web"
)

// homeQueryItem is one searchable Home entry. Text is the lowercased
// haystack (name + description, or title + host).
type homeQueryItem struct {
	Key  string
	Name string
	Text string
	URL  string
}

// homeQueryInput is everything the resolver needs; it holds no DOM state.
type homeQueryInput struct {
	Query      string
	EngineURL  string
	EngineName string
	Shortcuts  map[string]dto.HomeSearchShortcut
	Items      []homeQueryItem
}

// homeQueryResult describes what the Home search shows and opens.
type homeQueryResult struct {
	Mode       homeQueryMode
	EngineName string
	EngineURL  string
	// Matches are the keys of matching items, in input order.
	Matches []string
	// Target is what Enter opens; WebTarget is what Shift+Enter opens.
	Target    string
	WebTarget string
	// BangQuery is the typed bang key; Candidates are the sorted shortcut
	// keys starting with it (bang_partial and bang_unknown).
	BangQuery  string
	Candidates []string
}

// resolveHomeQuery interprets the Home search query. Targets that are not
// navigable from the privileged page are dropped.
func resolveHomeQuery(in homeQueryInput) homeQueryResult {
	engineURL := strings.TrimSpace(in.EngineURL)
	if engineURL == "" || !strings.Contains(engineURL, "%s") {
		engineURL = homeDefaultSearchEngine
	}
	engineName := strings.TrimSpace(in.EngineName)
	if engineName == "" || engineURL != strings.TrimSpace(in.EngineURL) {
		engineName = homeEngineHost(engineURL)
	}
	res := homeQueryResult{Mode: homeQueryIdle, EngineName: engineName, EngineURL: engineURL}

	raw := strings.TrimLeft(in.Query, " \t")
	query := strings.TrimSpace(raw)
	if query == "" {
		return res
	}
	res.WebTarget = homeEngineSearchURL(engineURL, query)

	if strings.HasPrefix(query, "!") {
		resolveHomeBang(&res, raw, in.Shortcuts)
		return res
	}

	needle := strings.ToLower(query)
	for _, item := range in.Items {
		if strings.Contains(item.Text, needle) {
			res.Matches = append(res.Matches, item.Key)
		}
	}
	firstMatch := ""
	for _, item := range in.Items {
		if len(res.Matches) > 0 && item.Key == res.Matches[0] {
			firstMatch = homeNavigable(item.URL)
			break
		}
	}

	if address := homeAddressURL(query); address != "" {
		res.Mode = homeQueryAddress
		res.WebTarget = address
		res.Target = address
		if firstMatch != "" {
			res.Target = firstMatch
		}
		return res
	}
	if len(res.Matches) > 0 {
		res.Mode = homeQueryFilter
		res.Target = firstMatch
		return res
	}
	res.Mode = homeQueryWeb
	res.Target = res.WebTarget
	return res
}

// resolveHomeBang fills res for a query starting with "!". raw keeps a
// trailing space so a finished key is not offered as a partial.
func resolveHomeBang(res *homeQueryResult, raw string, shortcuts map[string]dto.HomeSearchShortcut) {
	key, query, ok := browserurl.ParseBangPrefix(raw)
	if !ok {
		res.Mode = homeQueryBangUnknown
		res.Target = res.WebTarget
		return
	}
	res.BangQuery = key
	for name, sc := range shortcuts {
		if !strings.EqualFold(name, key) {
			continue
		}
		res.Mode = homeQueryBang
		res.EngineName = sc.Name
		res.EngineURL = ""
		if strings.Contains(sc.URL, "%s") && homeFillTemplate(sc.URL, "") != "" {
			res.EngineURL = sc.URL
		}
		if query != "" && res.EngineURL != "" {
			res.Target = homeFillTemplate(sc.URL, query)
		}
		return
	}
	lowerKey := strings.ToLower(key)
	for name := range shortcuts {
		if strings.HasPrefix(strings.ToLower(name), lowerKey) {
			res.Candidates = append(res.Candidates, name)
		}
	}
	sort.Strings(res.Candidates)
	res.Target = res.WebTarget
	if len(res.Candidates) > 0 && !strings.Contains(raw, " ") {
		res.Mode = homeQueryBangPartial
		return
	}
	res.Mode = homeQueryBangUnknown
}

// homeNavigable returns raw when the Home page may open it, else "".
func homeNavigable(raw string) string {
	if raw != "" && isHomeNavigableURL(raw) {
		return raw
	}
	return ""
}

// homeEngineHost is the engine display fallback: its host without "www.".
func homeEngineHost(template string) string {
	u, err := url.Parse(template)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
