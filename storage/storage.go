// Package storage provides interface to `pass` storage.
package storage

import (
	"bytes"
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if len(gpgIDs) == 0 {
		return "", errors.New("no GPG key configured")
	}

	// Encrypt with GPG - add all recipients
	args := []string{"--encrypt", "--batch", "--yes", "--output", fullPath, "--armor"}
	for _, gpgID := range gpgIDs {
		args = append(args, "--recipient", gpgID)
	}

	cmd := exec.Command("gpg", args...)
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

type Subscriber func(status string)

type Storage struct {
	path        string
	key         string
	passwords   []passcard.StoredItem
	subscribers []Subscriber
	cache       map[string]string
	cacheMutex  sync.RWMutex
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
	if query == "" {
		return s.passwords
	}

	var hits []passcard.StoredItem
	lowerQuery := strings.ToLower(query)
	queryParts := strings.Split(lowerQuery, " ")

	for _, p := range s.passwords {
		lowerName := strings.ToLower(p.Name)
		match := true
		for _, part := range queryParts {
			if !strings.Contains(lowerName, part) {
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
	oldPaths := make(map[string]bool)
	for _, p := range s.passwords {
		oldPaths[p.Path] = true
	}

	s.passwords = nil
	if err := filepath.Walk(s.path, s.index); err != nil {
		return
	}

	newPaths := make(map[string]bool)
	for _, p := range s.passwords {
		newPaths[p.Path] = true
	}

	s.cacheMutex.Lock()
	for path := range s.cache {
		if !newPaths[path] {
			delete(s.cache, path)
		}
	}
	s.cacheMutex.Unlock()

	s.publishUpdate(fmt.Sprintf("Indexed %d pass entries", len(s.passwords)))
}

func (s *Storage) index(path string, info os.FileInfo, err error) error {
	if strings.HasSuffix(path, ".gpg") {
		name := strings.TrimPrefix(path, s.path)
		name = strings.TrimSuffix(name, ".gpg")
		name = strings.TrimPrefix(name, "/")
		const MaxLen = 40
		if len(name) > MaxLen {
			name = "..." + name[len(name)-MaxLen:]
		}
		s.passwords = append(s.passwords, passcard.StoredItem{
			Name:    name,
			Path:    path,
			Storage: s,
		})
	}
	return nil
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
