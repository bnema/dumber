package favicon

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeIconCachePutWrapsErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := NewHomeIconCache(filepath.Join(file, "home")).Put(context.Background(), "https://a.lan", []byte("png"))
	if err == nil || !strings.HasPrefix(err.Error(), "home icon cache: create dir: ") {
		t.Fatalf("err = %v", err)
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("err %v does not wrap the os error", err)
	}
}

func TestHomeIconCachePutGetRoundTrip(t *testing.T) {
	c := NewHomeIconCache(t.TempDir())
	if err := c.Put(context.Background(), "https://a.lan/x", []byte("png")); err != nil {
		t.Fatal(err)
	}
	data, _, ok := c.Get(context.Background(), "https://A.lan/x")
	if !ok || string(data) != "png" {
		t.Fatalf("get = %q %v", data, ok)
	}
}
