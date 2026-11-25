// Package homehttp implements the HTTP client used by home page widgets.
package homehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/port"
)

const (
	// MaxResponseBytes is the largest response body accepted.
	MaxResponseBytes = 2 << 20
	// DefaultTimeout applies when the request has no timeout.
	DefaultTimeout = 10 * time.Second
	// MaxTimeout caps the request timeout.
	MaxTimeout = 60 * time.Second
	// MaxRedirects is the number of redirects followed.
	MaxRedirects = 3
	userAgent    = "dumber-home"
)

var (
	// ErrResponseTooLarge is returned when the body exceeds MaxResponseBytes.
	ErrResponseTooLarge = errors.New("home http: response body exceeds 2 MiB")
	// ErrUnsupportedScheme is returned for non-http(s) URLs or redirects.
	ErrUnsupportedScheme = errors.New("home http: only http and https URLs are allowed")
	// ErrTooManyRedirects is returned after MaxRedirects redirects.
	ErrTooManyRedirects = errors.New("home http: too many redirects")
)

// Client implements port.HomeHTTPClient. Private and LAN addresses are
// allowed because widgets target user-configured services. It keeps no cookies.
type Client struct {
	http *http.Client
}

var _ port.HomeHTTPClient = (*Client)(nil)

// New creates a client.
func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Client{http: &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > MaxRedirects {
				return ErrTooManyRedirects
			}
			if !isHTTPScheme(req.URL) {
				return ErrUnsupportedScheme
			}
			return nil
		},
	}}
}

// Do performs req. Errors name the method and the URL without its query and
// never include header values.
func (c *Client) Do(ctx context.Context, req port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
	target, err := url.Parse(req.URL)
	if err != nil {
		return port.HomeHTTPResponse{}, errors.New("home http: invalid URL")
	}
	if !isHTTPScheme(target) || target.Host == "" {
		return port.HomeHTTPResponse{}, ErrUnsupportedScheme
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return port.HomeHTTPResponse{}, fmt.Errorf("home http: build %s request: invalid method", method)
	}
	for name, value := range req.Headers {
		if strings.EqualFold(name, "Host") {
			httpReq.Host = value
			continue
		}
		httpReq.Header.Set(name, value)
	}
	httpReq.Header.Set("User-Agent", userAgent)

	where := method + " " + redactedURL(target)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return port.HomeHTTPResponse{}, fmt.Errorf("home http: %s: %w", where, unwrapURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return port.HomeHTTPResponse{}, fmt.Errorf("home http: %s: read body: %w", where, unwrapURLError(err))
	}
	if len(data) > MaxResponseBytes {
		return port.HomeHTTPResponse{}, fmt.Errorf("%w: %s", ErrResponseTooLarge, where)
	}
	return port.HomeHTTPResponse{StatusCode: resp.StatusCode, Body: data}, nil
}

func isHTTPScheme(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https")
}

// redactedURL drops user info, query and fragment, which may hold secrets.
func redactedURL(u *url.URL) string {
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// unwrapURLError drops the *url.Error wrapper, whose message repeats the full
// URL including its query.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
