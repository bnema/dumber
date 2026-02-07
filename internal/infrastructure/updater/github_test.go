package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// Equal versions
		{"equal simple", "1.0.0", "1.0.0", 0},
		{"equal two parts", "1.0", "1.0", 0},
		{"equal one part", "1", "1", 0},

		// v1 < v2 (should update)
		{"major less", "1.0.0", "2.0.0", -1},
		{"minor less", "1.1.0", "1.2.0", -1},
		{"patch less", "1.1.1", "1.1.2", -1},
		{"complex less", "0.20.1", "0.21.0", -1},
		{"real world less", "0.20.1", "0.20.2", -1},

		// v1 > v2 (no update needed)
		{"major greater", "2.0.0", "1.0.0", 1},
		{"minor greater", "1.2.0", "1.1.0", 1},
		{"patch greater", "1.1.2", "1.1.1", 1},

		// Pre-release versions (suffix stripped)
		{"prerelease equal", "1.0.0-alpha", "1.0.0", 0},
		{"prerelease less", "1.0.0-alpha", "1.0.1", -1},
		{"prerelease greater", "1.0.1-beta", "1.0.0", 1},

		// Partial versions
		{"partial v1", "1", "1.0.0", 0},
		{"partial v2", "1.0.0", "1", 0},
		{"partial less", "1", "2.0.0", -1},
		{"partial greater", "2", "1.0.0", 1},

		// Edge cases
		{"zero versions", "0.0.0", "0.0.0", 0},
		{"zero less", "0.0.0", "0.0.1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestGetArchName(t *testing.T) {
	// This test just ensures the function doesn't panic and returns something.
	arch := getArchName()
	if arch == "" {
		t.Error("getArchName() returned empty string")
	}
}

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		// Valid URLs
		{
			name:    "valid latest release URL",
			url:     "https://github.com/bnema/dumber/releases/latest/download/dumber_linux_x86_64.tar.gz",
			wantErr: false,
		},
		{
			name:    "valid versioned release URL",
			url:     "https://github.com/bnema/dumber/releases/download/v0.15.0/dumber_linux_x86_64.tar.gz",
			wantErr: false,
		},

		// Invalid scheme
		{
			name:    "http not allowed",
			url:     "http://github.com/bnema/dumber/releases/latest/download/dumber_linux_x86_64.tar.gz",
			wantErr: true,
			errMsg:  "must use HTTPS",
		},
		{
			name:    "ftp not allowed",
			url:     "ftp://github.com/bnema/dumber/releases/latest/download/file.tar.gz",
			wantErr: true,
			errMsg:  "must use HTTPS",
		},

		// Invalid host
		{
			name:    "wrong host",
			url:     "https://evil.com/bnema/dumber/releases/latest/download/file.tar.gz",
			wantErr: true,
			errMsg:  "must be from github.com",
		},
		{
			name:    "subdomain not allowed",
			url:     "https://raw.github.com/bnema/dumber/releases/latest/download/file.tar.gz",
			wantErr: true,
			errMsg:  "must be from github.com",
		},
		{
			name:    "typosquat domain",
			url:     "https://githuh.com/bnema/dumber/releases/latest/download/file.tar.gz",
			wantErr: true,
			errMsg:  "must be from github.com",
		},

		// Invalid path
		{
			name:    "not a releases URL",
			url:     "https://github.com/bnema/dumber/raw/main/README.md",
			wantErr: true,
			errMsg:  "must be a GitHub releases URL",
		},
		{
			name:    "blob URL not allowed",
			url:     "https://github.com/bnema/dumber/blob/main/cmd/dumber/main.go",
			wantErr: true,
			errMsg:  "must be a GitHub releases URL",
		},

		// Malformed URLs
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			url:     "://not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateDownloadURL(%q) expected error, got nil", tt.url)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateDownloadURL(%q) error = %q, want to contain %q", tt.url, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateDownloadURL(%q) unexpected error: %v", tt.url, err)
				}
			}
		})
	}
}

func TestSanitizeTarPath(t *testing.T) {
	destDir := "/tmp/extract"

	tests := []struct {
		name     string
		path     string
		wantErr  bool
		errMsg   string
		wantPath string
	}{
		// Valid paths
		{
			name:     "simple filename",
			path:     "dumber",
			wantErr:  false,
			wantPath: "/tmp/extract/dumber",
		},
		{
			name:     "nested path",
			path:     "dumber_v0.15.0/dumber",
			wantErr:  false,
			wantPath: "/tmp/extract/dumber_v0.15.0/dumber",
		},
		{
			name:     "deep nested path",
			path:     "release/bin/dumber",
			wantErr:  false,
			wantPath: "/tmp/extract/release/bin/dumber",
		},

		// Path traversal attempts
		{
			name:    "parent directory escape",
			path:    "../../../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name:    "hidden parent escape",
			path:    "dumber_v0.15.0/../../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name:     "double dot at start of filename",
			path:     "..dumber",
			wantErr:  false, // Valid filename, not a path traversal (.. is not a path component)
			wantPath: "/tmp/extract/..dumber",
		},
		{
			name:    "absolute path unix",
			path:    "/etc/passwd",
			wantErr: true,
			errMsg:  "absolute path",
		},
		{
			name:    "dot dot in middle",
			path:    "foo/../../../bar",
			wantErr: true,
			errMsg:  "path traversal",
		},

		// Edge cases
		{
			name:     "current directory",
			path:     "./dumber",
			wantErr:  false,
			wantPath: "/tmp/extract/dumber",
		},
		{
			name:     "multiple slashes",
			path:     "dumber_v0.15.0//dumber",
			wantErr:  false,
			wantPath: "/tmp/extract/dumber_v0.15.0/dumber",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeTarPath(tt.path, destDir)
			if tt.wantErr {
				if err == nil {
					t.Errorf("sanitizeTarPath(%q, %q) expected error, got nil", tt.path, destDir)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("sanitizeTarPath(%q, %q) error = %q, want to contain %q", tt.path, destDir, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("sanitizeTarPath(%q, %q) unexpected error: %v", tt.path, destDir, err)
				} else if tt.wantPath != "" && result != tt.wantPath {
					t.Errorf("sanitizeTarPath(%q, %q) = %q, want %q", tt.path, destDir, result, tt.wantPath)
				}
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	// Create a temporary file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile")
	testContent := []byte("hello world\n")

	if err := os.WriteFile(testFile, testContent, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate expected SHA256 hash
	hash := sha256.Sum256(testContent)
	expectedHash := hex.EncodeToString(hash[:])

	tests := []struct {
		name     string
		filePath string
		hash     string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid checksum lowercase",
			filePath: testFile,
			hash:     expectedHash,
			wantErr:  false,
		},
		{
			name:     "valid checksum uppercase",
			filePath: testFile,
			hash:     hex.EncodeToString(hash[:]),
			wantErr:  false,
		},
		{
			name:     "invalid checksum",
			filePath: testFile,
			hash:     "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr:  true,
			errMsg:   "checksum mismatch",
		},
		{
			name:     "wrong length hash",
			filePath: testFile,
			hash:     "abc123",
			wantErr:  true,
			errMsg:   "checksum mismatch",
		},
		{
			name:     "file not found",
			filePath: filepath.Join(tmpDir, "nonexistent"),
			hash:     expectedHash,
			wantErr:  true,
			errMsg:   "failed to open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyChecksum(tt.filePath, tt.hash)
			if tt.wantErr {
				if err == nil {
					t.Errorf("verifyChecksum() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("verifyChecksum() error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("verifyChecksum() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSizeLimits(t *testing.T) {
	// Verify our size constants are sensible
	if maxArchiveSize <= 0 {
		t.Error("maxArchiveSize should be positive")
	}
	if maxBinarySize <= 0 {
		t.Error("maxBinarySize should be positive")
	}
	if minBinarySize <= 0 {
		t.Error("minBinarySize should be positive")
	}
	if minBinarySize >= maxBinarySize {
		t.Errorf("minBinarySize (%d) should be less than maxBinarySize (%d)", minBinarySize, maxBinarySize)
	}
	if maxBinarySize >= maxArchiveSize {
		t.Errorf("maxBinarySize (%d) should be less than maxArchiveSize (%d)", maxBinarySize, maxArchiveSize)
	}
}

func TestRetryDelayForAttempt(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{name: "first attempt", attempt: 1, want: retryBaseDelay},
		{name: "second attempt", attempt: 2, want: retryBaseDelay * 2},
		{name: "third attempt", attempt: 3, want: retryBaseDelay * 4},
		{name: "capped attempt", attempt: 6, want: retryMaxDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retryDelayForAttempt(tt.attempt, func(_ int64) int64 { return 0 })
			if got != tt.want {
				t.Fatalf("retryDelayForAttempt(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestRetryDelayForAttemptAddsJitter(t *testing.T) {
	got := retryDelayForAttempt(1, func(_ int64) int64 { return int64(50 * time.Millisecond) })
	want := retryBaseDelay + (50 * time.Millisecond)
	if got != want {
		t.Fatalf("retryDelayForAttempt with jitter = %v, want %v", got, want)
	}
}

func TestGitHubCheckerDoRequestWithRetry_RetryableStatus(t *testing.T) {
	var calls int32
	tr := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempt := atomic.AddInt32(&calls, 1)
		if attempt < maxRetryAttempts {
			return testResponse(http.StatusForbidden, "forbidden"), nil
		}
		return testResponse(http.StatusOK, `{"tag_name":"v1.2.3"}`), nil
	})

	checker := NewGitHubChecker()
	checker.client = &http.Client{Transport: tr}
	checker.randInt63 = func(_ int64) int64 { return 0 }
	checker.sleep = func(context.Context, time.Duration) error { return nil }

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, githubAPIURL, http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := checker.doRequestWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("doRequestWithRetry() unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if atomic.LoadInt32(&calls) != maxRetryAttempts {
		t.Fatalf("calls = %d, want %d", calls, maxRetryAttempts)
	}
}

func TestGitHubCheckerCheckForUpdate_TransientError(t *testing.T) {
	var calls int32
	tr := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return testResponse(http.StatusForbidden, "forbidden"), nil
	})

	checker := NewGitHubChecker()
	checker.client = &http.Client{Transport: tr}
	checker.randInt63 = func(_ int64) int64 { return 0 }
	checker.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := checker.CheckForUpdate(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, port.ErrUpdateCheckTransient) {
		t.Fatalf("expected transient error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != maxRetryAttempts {
		t.Fatalf("calls = %d, want %d", calls, maxRetryAttempts)
	}
}

func TestGitHubDownloaderDoRequestWithRetry_NoRetryOnClientError(t *testing.T) {
	var calls int32
	tr := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return testResponse(http.StatusNotFound, "not found"), nil
	})

	downloader := NewGitHubDownloader()
	downloader.client = &http.Client{Transport: tr}
	downloader.randInt63 = func(_ int64) int64 { return 0 }
	downloader.sleep = func(context.Context, time.Duration) error { return nil }

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, checksumsURLTemplate, http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := downloader.doRequestWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("doRequestWithRetry() unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

// contains checks if substr is in s (helper for error message checks).
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
