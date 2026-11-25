package favicon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
	"golang.org/x/net/html"
)

const (
	homeIconTimeout      = 5 * time.Second
	homeIconMaxRedirects = 3
	homeIconMaxHTML      = 512 << 10
	homeIconMaxManifest  = 256 << 10
	homeIconMaxBytes     = 1 << 20
)

// HomeIconSource discovers and fetches icons for user-configured home
// services. Public hosts are allowed. Private network addresses are allowed
// only on the service's own host:port (from the user's home.toml), so a
// service page cannot point discovery at other LAN hosts through icon links,
// manifest srcs or redirects. It must only be wired into the home service
// icon use case, never used for page-supplied URLs.
type HomeIconSource struct {
	client *http.Client
	// isPrivate classifies resolved addresses; a test seam.
	isPrivate func(netip.Addr) bool
}

var _ appport.HomeIconSource = (*HomeIconSource)(nil)

type homeIconOriginKey struct{}

// NewHomeIconSource creates a HomeIconSource with its own HTTP client.
func NewHomeIconSource() *HomeIconSource {
	s := &HomeIconSource{isPrivate: isPrivateAddr}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = s.dialContext
	// Every request must dial so the address check runs: a pooled
	// connection opened for one service must not serve another.
	transport.DisableKeepAlives = true
	s.client = &http.Client{
		Timeout:       homeIconTimeout,
		Transport:     transport,
		CheckRedirect: homeIconCheckRedirect,
	}
	return s
}

// withServiceOrigin records the service host:port allowed on private networks.
func withServiceOrigin(ctx context.Context, serviceURL string) (context.Context, error) {
	u, err := parseHTTPURL(serviceURL)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, homeIconOriginKey{}, normalizeOriginHostPort(u.Hostname(), u.Port(), u.Scheme)), nil
}

// dialContext resolves the host once and dials a checked address: private
// addresses only for the service's own host:port.
func (s *HomeIconSource) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	allowed, _ := ctx.Value(homeIconOriginKey{}).(string)
	privateOK := allowed != "" && allowed == normalizeOriginHostPort(host, port, "")
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			continue
		}
		if s.isPrivate(addr.Unmap()) && !privateOK {
			continue
		}
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
	}
	return nil, fmt.Errorf("%w: private address for %s is outside the service host", appport.ErrHomeIconNotFound, host)
}

func homeIconCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > homeIconMaxRedirects {
		return fmt.Errorf("stopped after %d redirects", homeIconMaxRedirects)
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
	}
	return nil
}

func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: invalid service url %q", appport.ErrHomeIconNotFound, raw)
	}
	return u, nil
}

// homeIconResponse is a fully read, closed HTTP response.
type homeIconResponse struct {
	body        []byte
	finalURL    *url.URL
	contentType string
}

// get performs a GET and returns the body truncated to limit (truncate=true)
// or rejected when larger (truncate=false). finalURL is set even on errors
// once a response was received.
func (s *HomeIconSource) get(ctx context.Context, u *url.URL, limit int64, truncate bool) (homeIconResponse, error) {
	out := homeIconResponse{finalURL: u}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return out, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && resp.Request.URL != nil {
		out.finalURL = resp.Request.URL
	}
	out.contentType = resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("%w: status %d for %s", appport.ErrHomeIconNotFound, resp.StatusCode, u)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return out, err
	}
	if int64(len(body)) > limit {
		if !truncate {
			return out, fmt.Errorf("%w: response exceeds %d bytes", appport.ErrHomeIconNotFound, limit)
		}
		body = body[:limit]
	}
	out.body = body
	return out, nil
}

// iconCandidate is one discovered icon with its ranking data.
type iconCandidate struct {
	url  string
	tier int // lower is better
	size int // largest declared edge; 0 when unknown
}

const (
	tierSVG = iota
	tierManifest
	tierAppleTouch
	tierSizedIcon
	tierIcon
	tierFaviconICO
)

// DiscoverIconURLs returns candidate icon URLs, best first. /favicon.ico is
// always the last candidate, even when the page itself cannot be loaded.
func (s *HomeIconSource) DiscoverIconURLs(ctx context.Context, serviceURL string) ([]string, error) {
	pageURL, err := parseHTTPURL(serviceURL)
	if err != nil {
		return nil, err
	}
	if ctx, err = withServiceOrigin(ctx, serviceURL); err != nil {
		return nil, err
	}
	var candidates []iconCandidate
	page, getErr := s.get(ctx, pageURL, homeIconMaxHTML, true)
	base := page.finalURL
	if getErr == nil {
		var manifestURL *url.URL
		candidates, manifestURL = parseHomeIconLinks(page.body, base)
		if manifestURL != nil {
			candidates = append(candidates, s.manifestCandidates(ctx, manifestURL)...)
		}
	}
	fallback := base.ResolveReference(&url.URL{Path: "/favicon.ico"})
	candidates = append(candidates, iconCandidate{url: fallback.String(), tier: tierFaviconICO})
	return rankCandidates(candidates), getErr
}

func rankCandidates(candidates []iconCandidate) []string {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].tier != candidates[j].tier {
			return candidates[i].tier < candidates[j].tier
		}
		return candidates[i].size > candidates[j].size
	})
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, dup := seen[c.url]; dup {
			continue
		}
		seen[c.url] = struct{}{}
		out = append(out, c.url)
	}
	return out
}

type homeIconLink struct{ rel, href, typ, sizes string }

// parseHomeIconLinks extracts icon candidates and the manifest URL from HTML.
func parseHomeIconLinks(body []byte, pageURL *url.URL) ([]iconCandidate, *url.URL) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, nil
	}
	base, links := collectHeadLinks(doc, pageURL)
	var out []iconCandidate
	var manifest *url.URL
	for _, l := range links {
		u, err := base.Parse(strings.TrimSpace(l.href))
		if strings.TrimSpace(l.href) == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		rels := strings.Fields(l.rel)
		if hasToken(rels, "manifest") {
			if manifest == nil {
				manifest = u
			}
			continue
		}
		if c, ok := classifyIconLink(l, rels, u); ok {
			out = append(out, c)
		}
	}
	return out, manifest
}

// collectHeadLinks returns the effective base URL and every <link> before <body>.
func collectHeadLinks(doc *html.Node, pageURL *url.URL) (*url.URL, []homeIconLink) {
	base := pageURL
	var links []homeIconLink
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "base":
				if u, err := pageURL.Parse(attr(n, "href")); err == nil && attr(n, "href") != "" {
					base = u
				}
			case "link":
				links = append(links, homeIconLink{
					rel:   strings.ToLower(attr(n, "rel")),
					href:  attr(n, "href"),
					typ:   strings.ToLower(strings.TrimSpace(attr(n, "type"))),
					sizes: attr(n, "sizes"),
				})
			case "body":
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return base, links
}

func classifyIconLink(l homeIconLink, rels []string, u *url.URL) (iconCandidate, bool) {
	switch {
	case hasToken(rels, "apple-touch-icon"), hasToken(rels, "apple-touch-icon-precomposed"):
		size := largestSize(l.sizes)
		if size == 0 {
			size = 180 // Apple default touch icon size.
		}
		return iconCandidate{url: u.String(), tier: tierAppleTouch, size: size}, true
	case hasToken(rels, "icon"):
		if l.typ == "image/svg+xml" || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
			return iconCandidate{url: u.String(), tier: tierSVG}, true
		}
		size := largestSize(l.sizes)
		if size > 0 {
			return iconCandidate{url: u.String(), tier: tierSizedIcon, size: size}, true
		}
		return iconCandidate{url: u.String(), tier: tierIcon}, true
	}
	return iconCandidate{}, false
}

type webManifest struct {
	Icons []struct {
		Src   string `json:"src"`
		Sizes string `json:"sizes"`
		Type  string `json:"type"`
	} `json:"icons"`
}

func (s *HomeIconSource) manifestCandidates(ctx context.Context, manifestURL *url.URL) []iconCandidate {
	resp, err := s.get(ctx, manifestURL, homeIconMaxManifest, false)
	if err != nil {
		return nil
	}
	return parseManifestIcons(resp.body, resp.finalURL)
}

func parseManifestIcons(body []byte, manifestURL *url.URL) []iconCandidate {
	var m webManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	var out []iconCandidate
	for _, icon := range m.Icons {
		if strings.TrimSpace(icon.Src) == "" {
			continue
		}
		u, err := manifestURL.Parse(strings.TrimSpace(icon.Src))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		size := largestSize(icon.Sizes)
		if strings.EqualFold(icon.Type, "image/svg+xml") || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
			size = 1 << 16 // Scalable: always the best manifest icon.
		}
		out = append(out, iconCandidate{url: u.String(), tier: tierManifest, size: size})
	}
	return out
}

// largestSize returns the largest edge from a sizes attribute such as
// "16x16 32x32" or "any". "any" means scalable.
func largestSize(sizes string) int {
	best := 0
	for _, tok := range strings.Fields(strings.ToLower(sizes)) {
		if tok == "any" {
			return 1 << 16
		}
		w, h, ok := strings.Cut(tok, "x")
		if !ok {
			continue
		}
		wi, errW := strconv.Atoi(w)
		hi, errH := strconv.Atoi(h)
		if errW != nil || errH != nil {
			continue
		}
		best = max(best, min(wi, hi))
	}
	return best
}

func hasToken(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// FetchIcon downloads one icon (max 1 MiB) and sniffs its content type.
// Private addresses are allowed only on serviceURL's host:port.
func (s *HomeIconSource) FetchIcon(ctx context.Context, serviceURL, iconURL string) (*appport.HomeIconImage, error) {
	u, err := parseHTTPURL(iconURL)
	if err != nil {
		return nil, err
	}
	if ctx, err = withServiceOrigin(ctx, serviceURL); err != nil {
		return nil, err
	}
	resp, err := s.get(ctx, u, homeIconMaxBytes, false)
	if err != nil {
		return nil, err
	}
	body := resp.body
	ct := sniffHomeIconType(body, resp.contentType)
	if ct == "" {
		return nil, fmt.Errorf("%w: unsupported icon at %s", appport.ErrHomeIconNotFound, iconURL)
	}
	return &appport.HomeIconImage{URL: iconURL, Bytes: body, ContentType: ct}, nil
}

// ReadLocalIcon reads an override icon file (max 1 MiB).
func (*HomeIconSource) ReadLocalIcon(_ context.Context, path string) (*appport.HomeIconImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, homeIconMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > homeIconMaxBytes {
		return nil, fmt.Errorf("%w: override icon exceeds %d bytes", appport.ErrHomeIconNotFound, homeIconMaxBytes)
	}
	ct := sniffHomeIconType(body, "")
	if ct == "" {
		return nil, fmt.Errorf("%w: unsupported override icon %s", appport.ErrHomeIconNotFound, path)
	}
	return &appport.HomeIconImage{URL: path, Bytes: body, ContentType: ct}, nil
}

// sniffHomeIconType returns a supported image type from the bytes, using the
// declared type only to break ties. Empty means unsupported.
func sniffHomeIconType(body []byte, declared string) string {
	if len(body) == 0 {
		return ""
	}
	candidates := []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/svg+xml"}
	if d := normalizeContentType(declared); d != "" {
		candidates = append([]string{d}, candidates...)
	}
	for _, ct := range candidates {
		if supportedFetchedContentType(ct) && bodyMatchesContentType(ct, body) {
			return ct
		}
	}
	return ""
}
