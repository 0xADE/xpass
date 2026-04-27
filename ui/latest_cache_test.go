package ui

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLatestSelectionCachePathUsesXDGCacheHome(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	path, err := latestSelectionCachePath()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(cacheDir, "ade", "xpass", "latest")
	if path != want {
		t.Fatalf("cache path = %q, want %q", path, want)
	}
}

func TestLatestSelectionCacheRoundTripAndExpiration(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	savedAt := time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC)
	selection := latestSelection{
		SavedAt: savedAt,
		Query:   "bank/card",
		Path:    "/password-store/bank/card.gpg",
	}

	if err := saveLatestSelectionToCache(selection); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadLatestSelectionFromCache(savedAt.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected cached selection")
	}
	if *loaded != selection {
		t.Fatalf("loaded selection = %#v, want %#v", *loaded, selection)
	}

	expired, err := loadLatestSelectionFromCache(savedAt.Add(4 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if expired != nil {
		t.Fatalf("expected expired selection to be ignored, got %#v", *expired)
	}
}

func TestLatestSelectionCacheEmptyQuery(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	savedAt := time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC)
	selection := latestSelection{
		SavedAt: savedAt,
		Query:   "",
		Path:    "/password-store/full/list/item.gpg",
	}

	if err := saveLatestSelectionToCache(selection); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadLatestSelectionFromCache(savedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected cached selection with empty query")
	}
	if loaded.Query != "" || loaded.Path != selection.Path || loaded.SavedAt != savedAt {
		t.Fatalf("loaded = %#v, want %#v", *loaded, selection)
	}
}
