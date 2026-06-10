// Package favicon provides favicon fetching and caching infrastructure.
package favicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	domainfavicon "github.com/bnema/dumber/internal/domain/favicon"
)

const (
	duckduckgoFaviconURL = "https://icons.duckduckgo.com/ip3/%s.ico"
	fetchTimeout         = 5 * time.Second
	maxFaviconBytes      = 1 << 20
	maxFaviconRedirects  = 3
	sniffLen             = 512
)

type fetchContextKey string

const allowedLocalOriginKey fetchContextKey = "allowed-local-origin"

type Fetcher struct {
	client        *http.Client
	duckDuckGoURL string
	maxBytes      int64
}

func NewFetcher() *Fetcher {
	f := &Fetcher{duckDuckGoURL: duckduckgoFaviconURL, maxBytes: maxFaviconBytes}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = f.safeDialContext
	f.client = &http.Client{Timeout: fetchTimeout, Transport: transport, CheckRedirect: f.checkRedirect}
	return f
}

func NewFetcherWithClient(client *http.Client, duckDuckGoURL string) *Fetcher {
	f := &Fetcher{client: client, duckDuckGoURL: duckDuckGoURL, maxBytes: maxFaviconBytes}
	if f.client == nil {
		f.client = &http.Client{Timeout: fetchTimeout}
	}
	f.client.Transport = f.safeTransport(f.client.Transport)
	f.client.CheckRedirect = f.checkRedirect
	if f.duckDuckGoURL == "" {
		f.duckDuckGoURL = duckduckgoFaviconURL
	}
	return f
}

func (f *Fetcher) safeTransport(base http.RoundTripper) http.RoundTripper {
	transport, ok := base.(*http.Transport)
	if !ok || transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
	}
	clone := transport.Clone()
	clone.DialContext = f.safeDialContext
	return clone
}

func (f *Fetcher) Fetch(ctx context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
	fetchURL, resolvedKey, source, err := f.resolveURL(req)
	if err != nil {
		return nil, err
	}
	data, ct, err := f.fetchURL(ctx, fetchURL, req.PageURL)
	if err != nil {
		return nil, err
	}
	return &appport.FaviconFetchedIcon{
		PageURL:     req.PageURL,
		IconURL:     fetchURL,
		ResolvedKey: resolvedKey,
		Bytes:       data,
		Source:      source,
		ContentType: ct,
	}, nil
}

// FetchDomain preserves the old DuckDuckGo-only API for legacy callers.
func (f *Fetcher) FetchDomain(ctx context.Context, domain string) ([]byte, error) {
	if strings.TrimSpace(domain) == "" {
		return nil, nil
	}
	got, err := f.Fetch(ctx, appport.FaviconFetchRequest{PageURL: "https://" + domain})
	if errors.Is(err, appport.ErrFaviconMiss) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return got.Bytes, nil
}

func (f *Fetcher) resolveURL(req appport.FaviconFetchRequest) (string, domainfavicon.Key, domainfavicon.Source, error) {
	if req.IconURL != "" {
		u, err := url.Parse(req.IconURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return "", "", "", appport.ErrFaviconMiss
		}
		if !allowedURLForPage(req.PageURL, u) {
			return "", "", "", appport.ErrFaviconMiss
		}
		return u.String(), "", domainfavicon.SourcePageDiscovery, nil
	}
	key, ok := domainfavicon.CanonicalHostKey(req.PageURL)
	if !ok {
		return "", "", "", appport.ErrFaviconMiss
	}
	return fmt.Sprintf(f.duckDuckGoURL, url.QueryEscape(string(key))), key, domainfavicon.SourceDuckDuckGo, nil
}

func (f *Fetcher) fetchURL(ctx context.Context, raw, pageURL string) ([]byte, string, error) {
	ctx = context.WithValue(ctx, allowedLocalOriginKey, allowedLocalOrigin(pageURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, http.NoBody)
	if err != nil {
		return nil, "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", appport.ErrFaviconMiss
	}
	limit := f.maxBytes
	if limit <= 0 {
		limit = maxFaviconBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("favicon response exceeds %d bytes", limit)
	}
	if len(body) == 0 {
		return nil, "", appport.ErrFaviconMiss
	}
	declared := normalizeContentType(resp.Header.Get("Content-Type"))
	if declared == "" || declared == "application/octet-stream" {
		declared = normalizeContentType(http.DetectContentType(body[:min(len(body), sniffLen)]))
	}
	if !supportedFetchedContentType(declared) || !bodyMatchesContentType(declared, body) {
		return nil, "", appport.ErrFaviconMiss
	}
	return body, declared, nil
}

func (*Fetcher) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxFaviconRedirects {
		return http.ErrUseLastResponse
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return http.ErrUseLastResponse
	}
	if !allowedURLForOrigin(req.Context(), req.URL) {
		return http.ErrUseLastResponse
	}
	return nil
}

func (*Fetcher) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	allowed := ctx.Value(allowedLocalOriginKey) == normalizeOriginHostPort(host, port, "")
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var dialIP net.IP
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if (!ok || isPrivateAddr(addr)) && !allowed {
			continue
		}
		dialIP = ip.IP
		break
	}
	if dialIP == nil {
		return nil, fmt.Errorf("no allowed resolved address for favicon host %s", host)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, net.JoinHostPort(dialIP.String(), port))
}

func supportedFetchedContentType(ct string) bool {
	switch ct {
	case "image/png", "image/x-icon", "image/vnd.microsoft.icon", "image/jpeg", "image/gif", "image/svg+xml", "image/webp":
		return true
	}
	return false
}

func bodyMatchesContentType(ct string, body []byte) bool {
	switch ct {
	case "image/png":
		return bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n"))
	case "image/jpeg":
		return len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff
	case "image/gif":
		return bytes.HasPrefix(body, []byte("GIF87a")) || bytes.HasPrefix(body, []byte("GIF89a"))
	case "image/x-icon", "image/vnd.microsoft.icon":
		return len(body) >= 4 && body[0] == 0 && body[1] == 0 && (body[2] == 1 || body[2] == 2) && body[3] == 0
	case "image/webp":
		return len(body) >= 12 && bytes.HasPrefix(body, []byte("RIFF")) && string(body[8:12]) == "WEBP"
	case "image/svg+xml":
		trimmed := bytes.TrimSpace(body)
		return bytes.HasPrefix(trimmed, []byte("<svg")) || (bytes.HasPrefix(trimmed, []byte("<?xml")) && bytes.Contains(trimmed, []byte("<svg")))
	default:
		return false
	}
}

func allowedURLForPage(pageURL string, iconURL *url.URL) bool {
	if !isInternalHost(iconURL.Hostname()) {
		return true
	}
	page, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	return isLocalhost(page.Hostname()) && sameOrigin(page, iconURL)
}

func allowedURLForOrigin(ctx context.Context, target *url.URL) bool {
	if !isInternalHost(target.Hostname()) {
		return true
	}
	allowed, _ := ctx.Value(allowedLocalOriginKey).(string)
	targetOrigin := normalizeOriginHostPort(target.Hostname(), target.Port(), target.Scheme)
	return allowed != "" && strings.EqualFold(allowed, targetOrigin)
}

func allowedLocalOrigin(pageURL string) string {
	page, err := url.Parse(pageURL)
	if err != nil || !isLocalhost(page.Hostname()) {
		return ""
	}
	return normalizeOriginHostPort(page.Hostname(), page.Port(), page.Scheme)
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		normalizeOriginHostPort(a.Hostname(), a.Port(), a.Scheme) == normalizeOriginHostPort(b.Hostname(), b.Port(), b.Scheme)
}

func normalizeOriginHostPort(host, port, scheme string) string {
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if port == "" {
		return strings.ToLower(host)
	}
	return strings.ToLower(net.JoinHostPort(host, port))
}

func isInternalHost(host string) bool {
	if isLocalhost(host) {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || isPrivateAddr(addr) {
			return true
		}
	}
	return false
}

func isLocalhost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

func isPrivateAddr(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified()
}
