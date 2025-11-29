package filtering

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRemoteVersion(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		etag           string
		lastModified   string
		wantVersion    string
		wantErr        bool
	}{
		{
			name:           "returns ETag when present",
			responseStatus: http.StatusOK,
			etag:           `"abc123"`,
			wantVersion:    `"abc123"`,
			wantErr:        false,
		},
		{
			name:           "returns Last-Modified when no ETag",
			responseStatus: http.StatusOK,
			lastModified:   "Sat, 29 Nov 2025 12:48:00 GMT",
			wantVersion:    "Sat, 29 Nov 2025 12:48:00 GMT",
			wantErr:        false,
		},
		{
			name:           "prefers ETag over Last-Modified",
			responseStatus: http.StatusOK,
			etag:           `"xyz789"`,
			lastModified:   "Sat, 29 Nov 2025 12:48:00 GMT",
			wantVersion:    `"xyz789"`,
			wantErr:        false,
		},
		{
			name:           "error when no version headers",
			responseStatus: http.StatusOK,
			wantVersion:    "",
			wantErr:        true,
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			wantVersion:    "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify HEAD request
				assert.Equal(t, http.MethodHead, r.Method)

				if tt.etag != "" {
					w.Header().Set("ETag", tt.etag)
				}
				if tt.lastModified != "" {
					w.Header().Set("Last-Modified", tt.lastModified)
				}
				w.WriteHeader(tt.responseStatus)
			}))
			defer server.Close()

			fu := &FilterUpdater{
				httpClient: &http.Client{Timeout: 5 * time.Second},
			}

			version, err := fu.fetchRemoteVersion(context.Background(), server.URL)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		name           string
		storedVersion  string
		remoteETag     string
		serverError    bool
		expectedResult bool
	}{
		{
			name:           "etags differ - needs update",
			storedVersion:  `"old-etag"`,
			remoteETag:     `"new-etag"`,
			expectedResult: true,
		},
		{
			name:           "etags same - no update",
			storedVersion:  `"same-etag"`,
			remoteETag:     `"same-etag"`,
			expectedResult: false,
		},
		{
			name:           "no stored version - needs update",
			storedVersion:  "",
			remoteETag:     `"any-etag"`,
			expectedResult: true,
		},
		{
			name:           "server error - assume update needed",
			storedVersion:  `"old-etag"`,
			serverError:    true,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverError {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("ETag", tt.remoteETag)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			mockStore := &mockFilterStore{
				sourceVersions: map[string]string{
					server.URL: tt.storedVersion,
				},
			}

			fm := &FilterManager{store: mockStore}
			fu := &FilterUpdater{
				manager:    fm,
				httpClient: &http.Client{Timeout: 5 * time.Second},
			}

			result := fu.needsUpdate(context.Background(), server.URL)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// mockFilterStore implements FilterStore interface for testing
type mockFilterStore struct {
	sourceVersions map[string]string
	cachedFilters  *CompiledFilters
	cacheExists    bool
	cacheModTime   time.Time
	lastCheckTime  time.Time
}

func (m *mockFilterStore) LoadCached() (*CompiledFilters, error) {
	return m.cachedFilters, nil
}

func (m *mockFilterStore) SaveCache(filters *CompiledFilters) error {
	m.cachedFilters = filters
	return nil
}

func (m *mockFilterStore) GetCacheInfo() (bool, time.Time, error) {
	return m.cacheExists, m.cacheModTime, nil
}

func (m *mockFilterStore) GetSourceVersion(url string) string {
	if m.sourceVersions == nil {
		return ""
	}
	return m.sourceVersions[url]
}

func (m *mockFilterStore) SetSourceVersion(url string, version string) error {
	if m.sourceVersions == nil {
		m.sourceVersions = make(map[string]string)
	}
	m.sourceVersions[url] = version
	return nil
}

func (m *mockFilterStore) GetLastCheckTime() time.Time {
	return m.lastCheckTime
}
