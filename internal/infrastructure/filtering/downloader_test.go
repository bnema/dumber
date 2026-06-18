package filtering

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloader_DownloadFile_RecoversWhenTargetDirDeletedConcurrently(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	filename := filepath.Join("nested", "combined-part1.json")
	localPath := filepath.Join(cacheDir, filename)
	targetDir := filepath.Dir(localPath)

	require.NoError(t, os.MkdirAll(targetDir, cacheDirPerm))

	downloadStarted := make(chan struct{})
	allowResponse := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+filename {
			close(downloadStarted)
			<-allowResponse
			_, _ = io.WriteString(w, `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	type result struct {
		path string
		n    int64
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		path, n, err := d.downloadFile(context.Background(), filename, d.limits.maxFilterFileBytes)
		resultCh <- result{path: path, n: n, err: err}
	}()

	<-downloadStarted
	require.NoError(t, os.RemoveAll(targetDir))
	close(allowResponse)

	res := <-resultCh
	require.NoError(t, res.err)
	require.Equal(t, localPath, res.path)
	require.Positive(t, res.n)

	content, err := os.ReadFile(localPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "url-filter")
}

func TestDownloader_DownloadFilters_RejectsUnsafeManifestFilenames(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	requested := make([]string, 0, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		if r.URL.Path != "/"+FilterFiles.Manifest {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":1,"files":["../escape.json"]}}`)
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, paths)
	require.Contains(t, err.Error(), "invalid manifest")
	require.Equal(t, []string{"/" + FilterFiles.Manifest}, requested)
}

func TestDownloader_DownloadFilters_AllowsSafeNestedManifestFilenames(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	filename := "nested/combined-part1.json"
	body := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + FilterFiles.Manifest:
			_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":1,"files":["`+filename+`"]}}`)
		case "/" + filename:
			_, _ = io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.NoError(t, err)

	wantPath := filepath.Join(cacheDir, "nested", "combined-part1.json")
	require.Equal(t, []string{wantPath}, paths)
	content, err := os.ReadFile(wantPath)
	require.NoError(t, err)
	require.Equal(t, body, string(content))
}

func TestDownloader_DownloadFilters_PrunesObsoleteCachedCombinedParts(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "combined-part3.json"), []byte(`[]`), manifestFilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "other.json"), []byte(`[]`), manifestFilePerm))

	body := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + FilterFiles.Manifest:
			_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":2,"files":["combined-part1.json","combined-part2.json"]}}`)
		case "/combined-part1.json", "/combined-part2.json":
			_, _ = io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	require.FileExists(t, filepath.Join(cacheDir, "combined-part1.json"))
	require.FileExists(t, filepath.Join(cacheDir, "combined-part2.json"))
	require.NoFileExists(t, filepath.Join(cacheDir, "combined-part3.json"))
	require.FileExists(t, filepath.Join(cacheDir, "other.json"))
}

func TestDownloader_FetchManifest_RejectsOversizedManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+FilterFiles.Manifest {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, strings.Repeat("x", 17))
	}))
	defer server.Close()

	d := NewDownloader(t.TempDir())
	d.baseURL = server.URL
	d.limits.maxManifestBytes = 16

	manifest, err := d.FetchManifest(context.Background())
	require.Error(t, err)
	require.Nil(t, manifest)
	require.Contains(t, err.Error(), "manifest too large")
}

func TestDownloader_FetchManifest_RejectsTooManyFiles(t *testing.T) {
	payload, err := json.Marshal(Manifest{
		Version: "test",
		Combined: CombinedInfo{
			TotalRules: 3,
			Files:      []string{"part1.json", "part2.json", "part3.json"},
		},
	})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+FilterFiles.Manifest {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	d := NewDownloader(t.TempDir())
	d.baseURL = server.URL
	d.limits.maxFilterFiles = 2

	manifest, err := d.FetchManifest(context.Background())
	require.Error(t, err)
	require.Nil(t, manifest)
	require.Contains(t, err.Error(), "limit is 2")
}

func TestDownloader_NeedsUpdate_DetectsSameVersionManifestMetadataChange(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))

	cachedGeneratedAt := time.Date(2026, 3, 28, 4, 0, 0, 0, time.UTC)
	latestGeneratedAt := time.Date(2026, 3, 28, 5, 0, 0, 0, time.UTC)
	cachedManifest := Manifest{
		Version:     "2026.03.28",
		GeneratedAt: cachedGeneratedAt,
		Combined: CombinedInfo{
			TotalRules: 3,
			Files:      []string{"combined-part1.json", "combined-part2.json", "combined-part3.json"},
		},
	}
	cachedPayload, err := json.Marshal(cachedManifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, FilterFiles.Manifest), cachedPayload, manifestFilePerm))

	latestManifest := Manifest{
		Version:     "2026.03.28",
		GeneratedAt: latestGeneratedAt,
		Combined: CombinedInfo{
			TotalRules: 2,
			Files:      []string{"combined-part1.json", "combined-part2.json"},
		},
	}
	latestPayload, err := json.Marshal(latestManifest)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+FilterFiles.Manifest {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(latestPayload)
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	needsUpdate, err := d.NeedsUpdate(context.Background())
	require.NoError(t, err)
	require.True(t, needsUpdate)
}

func TestDownloader_DownloadFilters_RejectsOversizedFilterFile(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	filename := "combined.json"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + FilterFiles.Manifest:
			_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":1,"files":["combined.json"]}}`)
		case "/" + filename:
			_, _ = io.WriteString(w, "12345")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL
	d.limits.maxFilterFileBytes = 4
	d.limits.maxTotalFilterBytes = 32

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, paths)
	require.Contains(t, err.Error(), "too large")
	require.NoFileExists(t, filepath.Join(cacheDir, filename))
}

func TestDownloader_DownloadFilters_EnforcesTotalDownloadLimit(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + FilterFiles.Manifest:
			_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":2,"files":["part1.json","part2.json"]}}`)
		case "/part1.json", "/part2.json":
			_, _ = io.WriteString(w, "12345")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL
	d.limits.maxFilterFileBytes = 8
	d.limits.maxTotalFilterBytes = 8

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, paths)
	require.Contains(t, err.Error(), "too large")
	require.FileExists(t, filepath.Join(cacheDir, "part1.json"))
	require.NoFileExists(t, filepath.Join(cacheDir, "part2.json"))
}

func TestDownloader_DownloadFilters_RejectsSymlinkEscape(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	outsideDir := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))
	require.NoError(t, os.MkdirAll(outsideDir, cacheDirPerm))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(cacheDir, "nested")))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + FilterFiles.Manifest:
			_, _ = io.WriteString(w, `{"version":"test","combined":{"total_rules":1,"files":["nested/combined.json"]}}`)
		case "/nested/combined.json":
			_, _ = io.WriteString(w, `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewDownloader(cacheDir)
	d.baseURL = server.URL

	paths, err := d.DownloadFilters(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, paths)
	require.Contains(t, err.Error(), "failed to create target dir")
	require.NoFileExists(t, filepath.Join(outsideDir, "combined.json"))
}

func TestDownloader_GetCachedFilterPaths_RejectsSymlinkEscape(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	outsideDir := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))
	require.NoError(t, os.MkdirAll(outsideDir, cacheDirPerm))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(cacheDir, "nested")))
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "combined.json"), []byte("[]"), manifestFilePerm))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, FilterFiles.Manifest),
		[]byte(`{"version":"test","combined":{"total_rules":1,"files":["nested/combined.json"]}}`),
		manifestFilePerm,
	))

	d := NewDownloader(cacheDir)
	require.Nil(t, d.GetCachedFilterPaths())
}

func TestDownloader_RenameTempFile_RequiresDirectoryParent(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))

	tmpFile := filepath.Join(cacheDir, "payload.tmp")
	require.NoError(t, os.WriteFile(tmpFile, []byte("payload"), manifestFilePerm))

	parentAsFile := filepath.Join(cacheDir, "target")
	require.NoError(t, os.WriteFile(parentAsFile, []byte("not-a-directory"), manifestFilePerm))

	d := NewDownloader(cacheDir)

	err := d.renameTempFile(tmpFile, "target/filter.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create target dir")
}
