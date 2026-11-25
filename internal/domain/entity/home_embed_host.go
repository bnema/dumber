package entity

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

// IsBlockedEmbedURL reports whether an embed URL must be refused because
// its host is, or may be read by the browser as, HomeEmbedBlockedHost or one
// of its subdomains. The host is IDNA-mapped the way browsers do, so
// fullwidth letters or ideographic dots cannot bypass the check. Hosts with
// percent escapes, unparsable URLs and hosts that fail IDNA mapping are
// refused.
func IsBlockedEmbedURL(raw string) bool {
	if strings.Contains(homeRawAuthority(raw), "%") {
		return true
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return true
	}
	return IsBlockedEmbedHost(u.Hostname())
}

// IsBlockedEmbedHost reports whether host maps to HomeEmbedBlockedHost or a
// subdomain of it. See IsBlockedEmbedURL.
func IsBlockedEmbedHost(host string) bool {
	if host == "" || strings.Contains(host, "%") {
		return true
	}
	if net.ParseIP(host) != nil {
		return false
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return true
	}
	ascii = strings.TrimSuffix(strings.ToLower(ascii), ".")
	return ascii == "" || ascii == HomeEmbedBlockedHost || strings.HasSuffix(ascii, "."+HomeEmbedBlockedHost)
}

// homeRawAuthority returns the authority part of a raw URL, before parsing
// decodes it.
func homeRawAuthority(raw string) string {
	_, rest, ok := strings.Cut(strings.TrimSpace(raw), "//")
	if !ok {
		return ""
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
