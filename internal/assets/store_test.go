package assets

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
)

func TestFileAssetStorePutAndGetImageByAssetID(t *testing.T) {
	store, err := NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	meta, err := store.PutImage(context.Background(), PutImageInput{
		SessionID: corepkg.SessionID("sess-1"),
		ImageID:   2,
		MediaType: "image/png",
		FileName:  "a.png",
		Data:      []byte("png-bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.AssetID != llm.AssetID("sess-1:2") {
		t.Fatalf("unexpected asset id %q", meta.AssetID)
	}

	blob, err := store.GetImageByAssetID(context.Background(), corepkg.SessionID("sess-1"), meta.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if blob.MediaType != "image/png" {
		t.Fatalf("unexpected media type %q", blob.MediaType)
	}
	if !bytes.Equal(blob.Data, []byte("png-bytes")) {
		t.Fatalf("unexpected data %q", string(blob.Data))
	}
}

func TestFileAssetStoreRejectsUnsupportedImage(t *testing.T) {
	store, err := NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.PutImage(context.Background(), PutImageInput{
		SessionID: corepkg.SessionID("sess-1"),
		ImageID:   0,
		MediaType: "image/bmp",
		Data:      []byte("x"),
	})
	if err == nil {
		t.Fatal("expected unsupported image error")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != llm.ErrorCodeUnsupportedImage {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestFileAssetStoreDeleteSessionImages(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileAssetStore(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.PutImage(context.Background(), PutImageInput{
		SessionID: corepkg.SessionID("sess-1"),
		ImageID:   1,
		MediaType: "image/jpeg",
		Data:      []byte("jpeg"),
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteSessionImages(context.Background(), corepkg.SessionID("sess-1")); err != nil {
		t.Fatal(err)
	}

	_, err = store.GetImageByAssetID(context.Background(), corepkg.SessionID("sess-1"), llm.AssetID("sess-1:1"))
	if err == nil {
		t.Fatal("expected not found after deletion")
	}
}

func TestFileAssetStoreGetImageByAssetIDRejectsInvalidAssetID(t *testing.T) {
	store, err := NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.GetImageByAssetID(context.Background(), corepkg.SessionID("sess-1"), llm.AssetID("bad-id"))
	if err == nil {
		t.Fatal("expected invalid asset id to fail")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != llm.ErrorCodeAssetNotFound {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestFileAssetStoreRejectsCrossSessionAssetRead(t *testing.T) {
	store, err := NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	meta, err := store.PutImage(context.Background(), PutImageInput{
		SessionID: corepkg.SessionID("sess-a"),
		ImageID:   7,
		MediaType: "image/png",
		Data:      []byte("png"),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.GetImageByAssetID(context.Background(), corepkg.SessionID("sess-b"), meta.AssetID)
	if err == nil {
		t.Fatal("expected cross-session read to be blocked")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != llm.ErrorCodeAssetNotFound {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestFileAssetStoreRejectsMalformedSessionPrefixInAssetID(t *testing.T) {
	store, err := NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.PutImage(context.Background(), PutImageInput{
		SessionID: corepkg.SessionID("sess-a"),
		ImageID:   1,
		MediaType: "image/png",
		Data:      []byte("png"),
	}); err != nil {
		t.Fatal(err)
	}

	_, err = store.GetImageByAssetID(context.Background(), corepkg.SessionID("sess-a"), llm.AssetID(":1"))
	if err == nil {
		t.Fatal("expected malformed asset id to be blocked")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != llm.ErrorCodeAssetNotFound {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestFileAssetStoreGC(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileAssetStore(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.PutImage(context.Background(), PutImageInput{SessionID: corepkg.SessionID("keep"), ImageID: 1, MediaType: "image/png", Data: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutImage(context.Background(), PutImageInput{SessionID: corepkg.SessionID("drop"), ImageID: 2, MediaType: "image/png", Data: []byte("b")}); err != nil {
		t.Fatal(err)
	}

	dropDir := filepath.Join(root, "image-cache", "drop")
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dropDir, old, old); err != nil {
		t.Fatal(err)
	}

	if err := store.GC(context.Background(), []corepkg.SessionID{corepkg.SessionID("keep")}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dropDir); !os.IsNotExist(err) {
		t.Fatalf("expected drop dir removed, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "image-cache", "keep")); err != nil {
		t.Fatalf("expected keep dir still exists: %v", err)
	}
}
