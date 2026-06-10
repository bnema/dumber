package favicon

import (
	"net"
	"net/url"
	"path"
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

func CanonicalHostKey(raw string) (Key, bool) {
	candidates := hostCandidates(raw)
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0], true
}

func Candidates(raw string) []Key {
	parsed := parseHost(raw)
	if !parsed.ok {
		return nil
	}
	return withPathCandidates(hostCandidatesForParsed(parsed), parsed.rawPath)
}

func hostCandidates(raw string) []Key {
	parsed := parseHost(raw)
	if !parsed.ok {
		return nil
	}
	return hostCandidatesForParsed(parsed)
}

func hostCandidatesForParsed(parsed parsedHost) []Key {
	host := normalizeHost(parsed.host)
	if host == "" {
		return nil
	}

	var candidates []Key
	if isExactHost(host) {
		if parsed.port != "" && (!parsed.hasScheme || !isDefaultPort(parsed.scheme, parsed.port)) {
			return []Key{Key(net.JoinHostPort(host, parsed.port))}
		}
		return []Key{Key(host)}
	}

	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return nil
	}
	host = strings.ToLower(strings.TrimSuffix(ascii, "."))
	host = stripWWW(host)

	if parsed.port != "" {
		if !parsed.hasScheme || !isDefaultPort(parsed.scheme, parsed.port) {
			return []Key{Key(net.JoinHostPort(host, parsed.port))}
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

	for i := range len(labels) {
		candidate := strings.Join(labels[i:], ".")
		candidates = append(candidates, Key(candidate))
		if candidate == registrable {
			break
		}
	}
	return candidates
}

func withPathCandidates(hostCandidates []Key, rawPath string) []Key {
	if len(hostCandidates) == 0 {
		return nil
	}
	paths := pathCandidates(rawPath)
	if len(paths) == 0 {
		return hostCandidates
	}

	out := make([]Key, 0, len(paths)+len(hostCandidates))
	base := string(hostCandidates[0])
	for _, p := range paths {
		out = append(out, Key(base+p))
	}
	out = append(out, hostCandidates...)
	return out
}

func pathCandidates(rawPath string) []string {
	p := normalizePath(rawPath)
	if p == "" {
		return nil
	}

	var out []string
	for p != "" && p != "/" {
		out = append(out, p)
		idx := strings.LastIndex(p, "/")
		if idx <= 0 {
			break
		}
		p = p[:idx]
	}
	return out
}

func normalizePath(rawPath string) string {
	if rawPath == "" {
		return ""
	}
	if cut := strings.IndexAny(rawPath, "?#"); cut >= 0 {
		rawPath = rawPath[:cut]
	}
	if rawPath == "" || rawPath == "/" {
		return ""
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	cleaned := path.Clean(rawPath)
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	return strings.TrimRight(cleaned, "/")
}

type parsedHost struct {
	host      string
	port      string
	hasScheme bool
	scheme    string
	rawPath   string
	ok        bool
}

func parseHost(raw string) parsedHost {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedHost{}
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			return parsedHost{}
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return parsedHost{hasScheme: true, scheme: scheme}
		}
		host := u.Hostname()
		return parsedHost{
			host:      host,
			port:      u.Port(),
			hasScheme: true,
			scheme:    scheme,
			rawPath:   u.EscapedPath(),
			ok:        host != "",
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "about:") {
		return parsedHost{}
	}

	hostport := raw
	rawPath := ""
	if cut := strings.IndexAny(hostport, "/?#"); cut >= 0 {
		rawPath = hostport[cut:]
		hostport = hostport[:cut]
	}
	hostport = strings.TrimSuffix(hostport, ".")

	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return parsedHost{host: h, port: p, rawPath: rawPath, ok: h != ""}
	}
	if strings.Count(hostport, ":") == 1 {
		if h, p, ok := strings.Cut(hostport, ":"); ok && h != "" && p != "" {
			return parsedHost{host: h, port: p, rawPath: rawPath, ok: true}
		}
	}
	return parsedHost{host: hostport, rawPath: rawPath, ok: hostport != ""}
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
