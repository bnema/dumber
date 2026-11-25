package homehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"bnema.dev/dumber/internal/application/port"
)

// Prober implements port.HomeStatusProber. It sends HEAD, falling back to GET
// when the server rejects HEAD, follows no redirect, sends no custom headers
// and keeps no cookies. TLS verification stays enabled.
type Prober struct {
	http *http.Client
}

var _ port.HomeStatusProber = (*Prober)(nil)

// NewProber creates a prober.
func NewProber() *Prober {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Prober{http: &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// Probe checks rawURL. A certificate that fails verification counts as an
// answer (TLSUnverified). Errors name the scheme and host only.
func (p *Prober) Probe(ctx context.Context, rawURL string, timeout time.Duration) (port.HomeProbeResult, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return port.HomeProbeResult{}, errors.New("home probe: invalid URL")
	}
	if !isHTTPScheme(target) || target.Host == "" {
		return port.HomeProbeResult{}, ErrUnsupportedScheme
	}
	// Userinfo would make net/http send Authorization: Basic on its own.
	target.User = nil
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	code, err := p.send(ctx, http.MethodHead, target)
	if err == nil && (code == http.StatusMethodNotAllowed || code == http.StatusNotImplemented) {
		// Time only the GET, not the rejected HEAD round trip.
		start = time.Now()
		code, err = p.send(ctx, http.MethodGet, target)
	}
	latency := time.Since(start)
	if err != nil {
		if isCertVerificationError(err) {
			return port.HomeProbeResult{Latency: latency, TLSUnverified: true}, nil
		}
		return port.HomeProbeResult{}, fmt.Errorf("home probe: %s: %w", redactedURL(target), unwrapURLError(err))
	}
	return port.HomeProbeResult{StatusCode: code, Latency: latency}, nil
}

// send issues one request and closes the body without reading it.
func (p *Prober) send(ctx context.Context, method string, target *url.URL) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, target.String(), http.NoBody)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func isCertVerificationError(err error) bool {
	var (
		unknown  x509.UnknownAuthorityError
		invalid  x509.CertificateInvalidError
		hostname x509.HostnameError
		verify   *tls.CertificateVerificationError
	)
	if errors.As(err, &verify) {
		err = verify.Err
	}
	return errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &hostname)
}
