// Package storage provides interface to `pass` storage.
package storage

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"0xADE/xpass/passcard"

	"github.com/rjeczalik/notify"
)

type Subscriber func(status string)

type Storage struct {
	path        string
	key         string
	passwords   []passcard.StoredItem
	subscribers []Subscriber
}

func Init(basePath, key string) (*Storage, error) {
	s := &Storage{key: key}
	if err := s.findPasswordStore(basePath); err != nil {
		return nil, err
	}

	s.IndexAll()
	s.watch()
	return s, nil
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

func (s *Storage) IndexAll() {
	s.passwords = nil
	if err := filepath.Walk(s.path, s.index); err != nil {
		return
	}
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
		s.passwords = append(s.passwords, passcard.StoredItem{Name: name, Path: path})
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
		for range c {
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
