package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const latestSelectionMaxAge = 3 * time.Minute

type latestSelection struct {
	SavedAt time.Time `json:"saved_at"`
	Query   string    `json:"query"`
	Path    string    `json:"path"`
}

func latestSelectionCachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "ade", "xpass", "latest"), nil
}

func saveLatestSelectionToCache(selection latestSelection) error {
	if selection.SavedAt.IsZero() {
		selection.SavedAt = time.Now()
	}

	path, err := latestSelectionCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.Marshal(selection)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadLatestSelectionFromCache(now time.Time) (*latestSelection, error) {
	path, err := latestSelectionCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes only from latestSelectionCachePath() under UserCacheDir.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var selection latestSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return nil, nil
	}
	if selection.SavedAt.IsZero() || now.Sub(selection.SavedAt) > latestSelectionMaxAge {
		return nil, nil
	}
	if selection.Path == "" {
		return nil, nil
	}
	return &selection, nil
}
