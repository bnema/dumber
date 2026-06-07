package favicon

import (
	"bytes"
	"context"
	"errors"
	"testing"

	appport "github.com/bnema/dumber/internal/application/port"
	domainfavicon "github.com/bnema/dumber/internal/domain/favicon"
)

func TestBlobStoreWriteReadOverwriteAndInvalidate(t *testing.T) {
	ctx := context.Background()
	store := NewBlobStore(t.TempDir())
	key := domainfavicon.Key("example.com")

	if err := store.WriteOriginal(ctx, key, []byte("ico1"), "image/x-icon"); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteOriginal(ctx, key, []byte("ico2"), "image/png"); err != nil {
		t.Fatal(err)
	}
	got, ct, err := store.ReadOriginal(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ico2" || ct != "image/png" {
		t.Fatalf("original = %q %q", got, ct)
	}

	if err = store.WritePNG(ctx, key, []byte("png")); err != nil {
		t.Fatal(err)
	}
	if err = store.WriteSizedPNG(ctx, key, 32, []byte("sized")); err != nil {
		t.Fatal(err)
	}
	if got, _, err = store.ReadPNG(ctx, key); err != nil || !bytes.Equal(got, []byte("png")) {
		t.Fatalf("png = %q %v", got, err)
	}
	if got, _, err = store.ReadSizedPNG(ctx, key, 32); err != nil || !bytes.Equal(got, []byte("sized")) {
		t.Fatalf("sized = %q %v", got, err)
	}
	if err = store.RemoveDerived(ctx, key); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.ReadPNG(ctx, key)
	if !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("ReadPNG err = %v", err)
	}
	_, _, err = store.ReadSizedPNG(ctx, key, 32)
	if !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("ReadSizedPNG err = %v", err)
	}
	if got, _, err = store.ReadOriginal(ctx, key); err != nil || string(got) != "ico2" {
		t.Fatalf("original after invalidate = %q %v", got, err)
	}
}

func TestBlobStoreRemoveDerivedNoopsWhenDiskCacheDisabled(t *testing.T) {
	store := NewBlobStore("")
	if err := store.RemoveDerived(context.Background(), "example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestBlobStoreRemoveDerivedDoesNotRemoveSimilarDomainKeys(t *testing.T) {
	ctx := context.Background()
	store := NewBlobStore(t.TempDir())
	if err := store.WriteSizedPNG(ctx, "example.com", 32, []byte("example")); err != nil {
		t.Fatal(err)
	}
	if err := store.WritePNG(ctx, "example.com.au", []byte("other-png")); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSizedPNG(ctx, "example.com.au", 32, []byte("other-sized")); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveDerived(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReadSizedPNG(ctx, "example.com", 32); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("example.com sized err = %v", err)
	}
	if got, _, err := store.ReadPNG(ctx, "example.com.au"); err != nil || string(got) != "other-png" {
		t.Fatalf("example.com.au png = %q, %v", got, err)
	}
	if got, _, err := store.ReadSizedPNG(ctx, "example.com.au", 32); err != nil || string(got) != "other-sized" {
		t.Fatalf("example.com.au sized = %q, %v", got, err)
	}
}
