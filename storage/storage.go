// Package storage provides interface to `pass` storage.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"0xADE/xpass/passcard"

	"github.com/rjeczalik/notify"
)

func (s *Storage) Create(name string, content string, gpgIDs []string) (string, error) {
	fullPath := filepath.Join(s.path, name+".gpg")

	// Create subdirectories if they don't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if len(gpgIDs) == 0 {
		return "", errors.New("no GPG key configured")
	}

	if err := s.BackupGPGBeforeOverwrite(fullPath); err != nil {
		return "", err
	}

	// Encrypt with GPG - add all recipients
	args := []string{"--encrypt", "--batch", "--yes", "--output", fullPath, "--armor"}
	for _, gpgID := range gpgIDs {
		args = append(args, "--recipient", gpgID)
	}

	cmd := exec.CommandContext(context.Background(), "gpg", args...) //nolint:gosec // G204: fixed gpg flags; paths under password store.
	cmd.Stdin = strings.NewReader(content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to encrypt: %v: %s", err, stderr.String())
	}

	// The watcher should pick up the change, but for immediate UI update, we can re-index here.
	s.IndexAll()

	return fullPath, nil
}

// BackupGPGBeforeOverwrite writes an Emacs-style backup (path + "~") with a
// byte-for-byte copy of the current encrypted file when targetPath exists as
// a regular *.gpg entry under the store. Overwrites an existing backup. No-op
// if the entry file is missing. Does nothing (returns nil) if targetPath does
// not end with ".gpg".
func (s *Storage) BackupGPGBeforeOverwrite(targetPath string) error {
	cleanTarget := filepath.Clean(targetPath)
	cleanRoot := filepath.Clean(s.path)
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path is outside of storage")
	}
	if !strings.HasSuffix(cleanTarget, ".gpg") {
		return nil
	}
	fi, err := os.Stat(cleanTarget)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat entry: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("entry is not a regular file")
	}
	data, err := os.ReadFile(cleanTarget)
	if err != nil {
		return fmt.Errorf("read entry for backup: %w", err)
	}
	backupPath := cleanTarget + "~"
	if err := os.WriteFile(backupPath, data, fi.Mode().Perm()); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// Delete removes an entry file under the password store and re-indexes.
func (s *Storage) Delete(targetPath string) error {
	cleanTarget := filepath.Clean(targetPath)
	cleanRoot := filepath.Clean(s.path)
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path is outside of storage")
	}
	if err := os.Remove(cleanTarget); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	s.invalidateCache(cleanTarget)
	s.IndexAll()
	return nil
}

type Subscriber func(status string)

type Storage struct {
	path           string
	key            string
	passwords      []passcard.StoredItem
	passwordsMutex sync.RWMutex
	subscribers    []Subscriber
	cache          map[string]string
	cacheMutex     sync.RWMutex
}

func Init(basePath, key string) (*Storage, error) {
	s := &Storage{
		key:   key,
		cache: make(map[string]string),
	}
	if err := s.findPasswordStore(basePath); err != nil {
		return nil, err
	}

	s.IndexAll()
	s.watch()
	return s, nil
}

func (s *Storage) Path() string {
	return s.path
}

func (s *Storage) Query(query string) []passcard.StoredItem {
	s.passwordsMutex.RLock()
	defer s.passwordsMutex.RUnlock()

	if query == "" {
		out := make([]passcard.StoredItem, len(s.passwords))
		copy(out, s.passwords)
		return out
	}

	var hits []passcard.StoredItem
	lowerQuery := strings.ToLower(query)
	queryParts := strings.Split(lowerQuery, " ")

	for _, p := range s.passwords {
		lowerRel := strings.ToLower(p.RelPath)
		match := true
		for _, part := range queryParts {
			if part == "" {
				continue
			}
			if !strings.Contains(lowerRel, part) {
				match = false
				break
			}
		}
		if match {
			hits = append(hits, p)
		}
	}
	return hits
}

func (s *Storage) NameByIdx(idx int) string {
	s.passwordsMutex.RLock()
	defer s.passwordsMutex.RUnlock()
	if idx >= len(s.passwords) {
		return ""
	}
	return s.passwords[idx].Name
}

func (s *Storage) Subscribe(cb Subscriber) {
	s.subscribers = append(s.subscribers, cb)
}

func (s *Storage) publishUpdate(status string) {
	for _, sub := range s.subscribers {
		sub(status)
	}
}

func (s *Storage) GetCached(path string) (string, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	cached, ok := s.cache[path]
	return cached, ok
}

func (s *Storage) SetCached(path, value string) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.cache[path] = value
}

func (s *Storage) invalidateCache(path string) {
	if strings.HasSuffix(path, ".gpg") {
		s.cacheMutex.Lock()
		delete(s.cache, path)
		s.cacheMutex.Unlock()
	}
}

func (s *Storage) IndexAll() {
	var newPasswords []passcard.StoredItem
	walkFn := func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, "~") {
			return nil
		}
		if !strings.HasSuffix(path, ".gpg") {
			return nil
		}
		rel := strings.TrimPrefix(path, s.path)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		rel = strings.TrimSuffix(rel, ".gpg")
		relSlash := filepath.ToSlash(rel)

		displayName := relSlash
		const MaxLen = 40
		if len(displayName) > MaxLen {
			displayName = "..." + displayName[len(displayName)-MaxLen:]
		}
		newPasswords = append(newPasswords, passcard.StoredItem{
			Name:    displayName,
			RelPath: relSlash,
			Path:    path,
			Storage: s,
		})
		return nil
	}
	if err := filepath.Walk(s.path, walkFn); err != nil {
		return
	}

	newPaths := make(map[string]bool)
	for _, p := range newPasswords {
		newPaths[p.Path] = true
	}

	s.passwordsMutex.Lock()
	s.passwords = newPasswords
	s.passwordsMutex.Unlock()

	s.cacheMutex.Lock()
	for path := range s.cache {
		if !newPaths[path] {
			delete(s.cache, path)
		}
	}
	s.cacheMutex.Unlock()

	s.publishUpdate(fmt.Sprintf("Indexed %d pass entries", len(newPasswords)))
}

func (s *Storage) watch() {
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(s.path+"/...", c, notify.All); err != nil {
		log.Printf("Failed to watch password store: %v", err)
		return
	}

	go func() {
		for event := range c {
			s.invalidateCache(event.Path())
			s.IndexAll()
		}
	}()
}

func (s *Storage) findPasswordStore(basePath string) error {
	var homeDir string
	if usr, err := user.Current(); err == nil {
		homeDir = usr.HomeDir
	}
	pathCandidates := []string{
		basePath,
		path.Join(homeDir, ".password-store"),
		path.Join(homeDir, "password-store"),
	}
	for _, p := range pathCandidates {
		if p == "" {
			continue
		}
		var err error
		if p, err = filepath.EvalSymlinks(p); err != nil {
			continue
		}
		if _, err = os.Stat(p); err != nil {
			continue
		}
		s.path = p
		return nil
	}
	return errors.New("couldn't find a valid password store")
}
