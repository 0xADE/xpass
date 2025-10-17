// Package storage provides interface to `pass` storage.
package storage

import (
	"errors"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

type (
	Storage struct {
		path  string
		key   string
		items []item
	}
	item struct {
		path  string
		title string
	}
)

func Init(basePath, key string) (*Storage, error) {
	var err error
	s := &Storage{key: key}
	if s.findPasswordStore(basePath); err != nil {
		return nil, err
	}

	// XXX call it from main in goroutine
	s.IndexAll()
	// XXX ps.watch()
	return s, nil
}

func (s *Storage) IndexAll() {
	filepath.Walk(s.path, s.index)
}

func (s *Storage) index(path string, info os.FileInfo, err error) error {
	if strings.HasSuffix(path, ".gpg") {
		title := strings.TrimPrefix(path, s.path)
		title = strings.TrimSuffix(title, ".gpg")
		title = strings.TrimPrefix(title, "/")
		const MaxLen = 40
		if len(title) > MaxLen {
			title = "..." + title[len(title)-MaxLen:]
		}
		s.items = append(s.items, item{title: title, path: path})
	}
	return nil
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
