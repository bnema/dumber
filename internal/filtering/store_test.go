package filtering

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileFilterStore_SourceVersions(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "filter-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create store with temp paths
	store := &FileFilterStore{
		cacheFile: filepath.Join(tmpDir, "cache.json"),
		metaFile:  filepath.Join(tmpDir, "metadata.json"),
	}

	testURL := "https://easylist.to/easylist/easylist.txt"
	testVersion := "202511291248"

	t.Run("get version when no metadata exists", func(t *testing.T) {
		version := store.GetSourceVersion(testURL)
		assert.Empty(t, version)
	})

	t.Run("set and get version", func(t *testing.T) {
		err := store.SetSourceVersion(testURL, testVersion)
		require.NoError(t, err)

		version := store.GetSourceVersion(testURL)
		assert.Equal(t, testVersion, version)
	})

	t.Run("update existing version", func(t *testing.T) {
		newVersion := "202511301000"
		err := store.SetSourceVersion(testURL, newVersion)
		require.NoError(t, err)

		version := store.GetSourceVersion(testURL)
		assert.Equal(t, newVersion, version)
	})

	t.Run("multiple URLs", func(t *testing.T) {
		url1 := "https://example.com/list1.txt"
		url2 := "https://example.com/list2.txt"
		version1 := "111111111111"
		version2 := "222222222222"

		err := store.SetSourceVersion(url1, version1)
		require.NoError(t, err)
		err = store.SetSourceVersion(url2, version2)
		require.NoError(t, err)

		assert.Equal(t, version1, store.GetSourceVersion(url1))
		assert.Equal(t, version2, store.GetSourceVersion(url2))
	})

	t.Run("last check time is updated", func(t *testing.T) {
		beforeSet := time.Now().Add(-time.Second)

		err := store.SetSourceVersion(testURL, testVersion)
		require.NoError(t, err)

		lastCheck := store.GetLastCheckTime()
		assert.True(t, lastCheck.After(beforeSet))
	})
}

func TestCacheMetadata_SourceVersions(t *testing.T) {
	t.Run("nil map handling", func(t *testing.T) {
		metadata := &CacheMetadata{
			Version:        cacheVersion,
			SourceVersions: nil,
		}

		// Should handle nil map gracefully
		assert.Nil(t, metadata.SourceVersions)
	})

	t.Run("initialized map", func(t *testing.T) {
		metadata := &CacheMetadata{
			Version: cacheVersion,
			SourceVersions: map[string]string{
				"url1": "v1",
				"url2": "v2",
			},
		}

		assert.Equal(t, "v1", metadata.SourceVersions["url1"])
		assert.Equal(t, "v2", metadata.SourceVersions["url2"])
		assert.Empty(t, metadata.SourceVersions["nonexistent"])
	})
}
