package cef

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// DiscoverFaviconCandidates extracts ordered favicon URL candidates from page
// metadata and appends the origin /favicon.ico fallback. Only http(s) page and
// icon URLs are returned; duplicates are removed while preserving order.
func DiscoverFaviconCandidates(pageURL, htmlSource string) []string {
	candidates := make([]string, 0, 4)
	for _, href := range parseFaviconHrefs(htmlSource) {
		if resolved, ok := resolveFaviconURL(pageURL, href); ok {
			candidates = append(candidates, resolved)
		}
	}
	if fallback, ok := originFaviconURL(pageURL); ok {
		candidates = append(candidates, fallback)
	}
	if len(candidates) == 0 {
		return nil
	}
	return dedupeStrings(candidates)
}

func resolveFaviconCandidates(pageURL string, rawIconURLs []string) []string {
	candidates := make([]string, 0, len(rawIconURLs))
	for _, raw := range rawIconURLs {
		if resolved, ok := resolveFaviconURL(pageURL, raw); ok {
			candidates = append(candidates, resolved)
		}
	}
	return dedupeStrings(candidates)
}

func fallbackFaviconCandidates(pageURL string) []string {
	fallback, ok := originFaviconURL(pageURL)
	if !ok {
		return nil
	}
	return []string{fallback}
}

func parseFaviconHrefs(htmlSource string) []string {
	z := html.NewTokenizer(strings.NewReader(htmlSource))
	var hrefs []string
	for {
		switch z.Next() {
		case html.ErrorToken:
			return hrefs
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := z.TagName()
			if !hasAttr || !strings.EqualFold(string(tn), "link") {
				continue
			}
			var rel, href string
			for {
				key, val, more := z.TagAttr()
				switch strings.ToLower(string(key)) {
				case "rel":
					rel = string(val)
				case "href":
					href = string(val)
				}
				if !more {
					break
				}
			}
			if href != "" && faviconRel(rel) {
				hrefs = append(hrefs, href)
			}
		}
	}
}

func faviconRel(rel string) bool {
	fields := strings.FieldsSeq(strings.ToLower(rel))
	for field := range fields {
		if field == "icon" || field == "apple-touch-icon" || field == "apple-touch-icon-precomposed" {
			return true
		}
	}
	return false
}

func resolveFaviconURL(pageURL, rawIconURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawIconURL)
	if trimmed == "" {
		return "", false
	}
	icon, err := url.Parse(trimmed)
	if err != nil {
		return "", false
	}
	if icon.IsAbs() {
		if !isHTTPScheme(icon.Scheme) || icon.Host == "" {
			return "", false
		}
		return icon.String(), true
	}
	base, err := url.Parse(pageURL)
	if err != nil || !isHTTPScheme(base.Scheme) || base.Host == "" {
		return "", false
	}
	resolved := base.ResolveReference(icon)
	if !isHTTPScheme(resolved.Scheme) || resolved.Host == "" {
		return "", false
	}
	return resolved.String(), true
}

func originFaviconURL(pageURL string) (string, bool) {
	u, err := url.Parse(pageURL)
	if err != nil || !isHTTPScheme(u.Scheme) || u.Host == "" {
		return "", false
	}
	u.Path = "/favicon.ico"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), true
}

func isHTTPScheme(scheme string) bool {
	return scheme == "http" || scheme == "https"
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
