package entity

import (
	"net"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/idna"
)

// HomeEmbedOrigin returns the canonical origin (scheme://host[:port]) of an
// http(s) URL, the way the browser serializes it: lowercase scheme, IDNA
// ASCII lowercase host, IPv6 in brackets, default port elided. It reports
// false for other schemes, empty or unmappable hosts, and blocked embed
// hosts.
func HomeEmbedOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !u.IsAbs() {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	defaultPort := ""
	switch scheme {
	case "http":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	default:
		return "", false
	}
	if IsBlockedEmbedURL(raw) {
		return "", false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		ascii, err := idna.Lookup.ToASCII(host)
		if err != nil {
			return "", false
		}
		host = strings.TrimSuffix(strings.ToLower(ascii), ".")
	}
	if host == "" {
		return "", false
	}
	port := u.Port()
	if port == defaultPort {
		port = ""
	}
	if port == "" {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return scheme + "://" + host, true
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}

// EmbedOrigins returns the sorted, deduplicated origins of the layout's
// valid embed widgets.
func (l HomeLayout) EmbedOrigins() []string {
	seen := map[string]bool{}
	var origins []string
	for gi := range l.Groups {
		for si := range l.Groups[gi].Services {
			s := &l.Groups[gi].Services[si]
			if s.Widget == nil || s.Widget.Type != HomeServiceWidgetEmbed {
				continue
			}
			origin, ok := HomeEmbedOrigin(s.Widget.URL)
			if !ok || seen[origin] {
				continue
			}
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	sort.Strings(origins)
	return origins
}
