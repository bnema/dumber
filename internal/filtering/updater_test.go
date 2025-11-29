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
		responseBody   string
		responseStatus int
		wantVersion    string
		wantErr        bool
	}{
		{
			name: "valid version header",
			responseBody: `[Adblock Plus 2.0]
! Version: 202511291248
! Title: EasyList
! Last modified: 29 Nov 2025 12:48 UTC
`,
			responseStatus: http.StatusOK,
			wantVersion:    "202511291248",
			wantErr:        false,
		},
		{
			name: "partial content response (206)",
			responseBody: `[Adblock Plus 2.0]
! Version: 202501011200
! Title: Test
`,
			responseStatus: http.StatusPartialContent,
			wantVersion:    "202501011200",
			wantErr:        false,
		},
		{
			name:           "no version header",
			responseBody:   `[Adblock Plus 2.0]\n! Title: No Version\n`,
			responseStatus: http.StatusOK,
			wantVersion:    "",
			wantErr:        true,
		},
		{
			name:           "server error",
			responseBody:   "",
			responseStatus: http.StatusInternalServerError,
			wantVersion:    "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Range header is set
				rangeHeader := r.Header.Get("Range")
				assert.Contains(t, rangeHeader, "bytes=0-")

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create updater with test server
			fu := &FilterUpdater{
				httpClient: &http.Client{Timeout: 5 * time.Second},
			}

			// Test fetchRemoteVersion
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

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name: "standard format",
			content: []byte(`[Adblock Plus 2.0]
! Version: 202511291248
! Title: EasyList`),
			want: "202511291248",
		},
		{
			name: "version with spaces",
			content: []byte(`! Version:   20251129
! Title: Test`),
			want: "20251129",
		},
		{
			name:    "no version",
			content: []byte(`! Title: No Version\n! Author: Test`),
			want:    "",
		},
		{
			name:    "empty content",
			content: []byte{},
			want:    "",
		},
		{
			name: "version after 20 lines (not found)",
			content: []byte(`line1
line2
line3
line4
line5
line6
line7
line8
line9
line10
line11
line12
line13
line14
line15
line16
line17
line18
line19
line20
line21
! Version: 202511291248`),
			want: "",
		},
	}

	fu := &FilterUpdater{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fu.extractVersion(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		name           string
		storedVersion  string
		remoteVersion  string
		serverError    bool
		expectedResult bool
	}{
		{
			name:           "versions differ - needs update",
			storedVersion:  "202511280000",
			remoteVersion:  "202511291248",
			expectedResult: true,
		},
		{
			name:           "versions same - no update",
			storedVersion:  "202511291248",
			remoteVersion:  "202511291248",
			expectedResult: false,
		},
		{
			name:           "no stored version - needs update",
			storedVersion:  "",
			remoteVersion:  "202511291248",
			expectedResult: true,
		},
		{
			name:           "server error - assume update needed",
			storedVersion:  "202511280000",
			serverError:    true,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverError {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("! Version: " + tt.remoteVersion + "\n"))
			}))
			defer server.Close()

			// Create mock store
			mockStore := &mockFilterStore{
				sourceVersions: map[string]string{
					server.URL: tt.storedVersion,
				},
			}

			// Create manager and updater
			fm := &FilterManager{store: mockStore}
			fu := &FilterUpdater{
				manager:    fm,
				httpClient: &http.Client{Timeout: 5 * time.Second},
			}

			// Test needsUpdate
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
