package filtering

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
		path, n, err := d.downloadFile(context.Background(), filename)
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

func TestDownloader_RenameTempFile_RequiresDirectoryParent(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(cacheDir, cacheDirPerm))

	tmpFile := filepath.Join(cacheDir, "payload.tmp")
	require.NoError(t, os.WriteFile(tmpFile, []byte("payload"), manifestFilePerm))

	parentAsFile := filepath.Join(cacheDir, "target")
	require.NoError(t, os.WriteFile(parentAsFile, []byte("not-a-directory"), manifestFilePerm))

	d := NewDownloader(cacheDir)

	err := d.renameTempFile(tmpFile, filepath.Join(parentAsFile, "filter.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")
}
