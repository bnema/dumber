// Package favicon provides favicon fetching and caching infrastructure.
package favicon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

const (
	// DuckDuckGo favicon API URL template.
	duckduckgoFaviconURL = "https://icons.duckduckgo.com/ip3/%s.ico"
	// HTTP client timeout for favicon fetch.
	fetchTimeout = 5 * time.Second
)

// Fetcher retrieves favicons from external APIs.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new Fetcher with default HTTP client settings.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: fetchTimeout,
		},
	}
}

// Fetch retrieves favicon bytes for a domain from DuckDuckGo's favicon API.
// Returns nil bytes if the favicon cannot be fetched.
func (f *Fetcher) Fetch(ctx context.Context, domain string) ([]byte, error) {
	if domain == "" {
		return nil, nil
	}

	log := logging.FromContext(ctx)
	faviconURL := fmt.Sprintf(duckduckgoFaviconURL, url.QueryEscape(domain))

	log.Debug().Str("url", faviconURL).Msg("fetching favicon from DuckDuckGo")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, faviconURL, http.NoBody)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create favicon request")
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to fetch favicon from DuckDuckGo")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug().Int("status", resp.StatusCode).Str("domain", domain).Msg("DuckDuckGo favicon API returned non-OK status")
		return nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to read favicon response")
		return nil, err
	}

	if len(data) == 0 {
		log.Debug().Str("domain", domain).Msg("empty favicon response from DuckDuckGo")
		return nil, nil
	}

	log.Debug().Str("domain", domain).Int("bytes", len(data)).Msg("favicon fetched from DuckDuckGo")
	return data, nil
}
