package favicon

import (
	"net"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type Key string

func CanonicalKey(raw string) (Key, bool) {
	candidates := Candidates(raw)
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0], true
}

func Candidates(raw string) []Key {
	host, port, hasScheme, scheme, ok := parseHost(raw)
	if !ok {
		return nil
	}

	host = normalizeHost(host)
	if host == "" {
		return nil
	}

	if isExactHost(host) {
		if port != "" && (!hasScheme || !isDefaultPort(scheme, port)) {
			return []Key{Key(net.JoinHostPort(host, port))}
		}
		return []Key{Key(host)}
	}

	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return nil
	}
	host = strings.ToLower(strings.TrimSuffix(ascii, "."))
	host = stripWWW(host)

	if port != "" {
		if !hasScheme || !isDefaultPort(scheme, port) {
			return []Key{Key(net.JoinHostPort(host, port))}
		}
	}

	labels := strings.Split(host, ".")
	if slices.Contains(labels, "") {
		return nil
	}

	registrable, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return []Key{Key(host)}
	}

	var out []Key
	for i := range len(labels) {
		candidate := strings.Join(labels[i:], ".")
		out = append(out, Key(candidate))
		if candidate == registrable {
			break
		}
	}
	return out
}

func parseHost(raw string) (host, port string, hasScheme bool, scheme string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false, "", false
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			return "", "", false, "", false
		}
		scheme = strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return "", "", true, scheme, false
		}
		host = u.Hostname()
		port = u.Port()
		return host, port, true, scheme, host != ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "about:") {
		return "", "", false, "", false
	}

	hostport := raw
	if cut := strings.IndexAny(hostport, "/?#"); cut >= 0 {
		hostport = hostport[:cut]
	}
	hostport = strings.TrimSuffix(hostport, ".")

	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p, false, "", h != ""
	}
	if strings.Count(hostport, ":") == 1 {
		if h, p, ok := strings.Cut(hostport, ":"); ok && h != "" && p != "" {
			return h, p, false, "", true
		}
	}
	return hostport, "", false, "", hostport != ""
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

func stripWWW(host string) string {
	return strings.TrimPrefix(host, "www.")
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}

func isExactHost(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil
}
