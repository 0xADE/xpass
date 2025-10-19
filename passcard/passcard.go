// Package passcard parse single item (card) with password and
// supplementary information that provided in password storage.
package passcard

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
)

type CacheInterface interface {
	GetCached(path string) (string, bool)
	SetCached(path, value string)
}

type StoredItem struct {
	Name    string
	Path    string
	Storage CacheInterface
}

func (p *StoredItem) decrypt() (string, error) {
	if p.Storage != nil {
		if cached, ok := p.Storage.GetCached(p.Path); ok {
			return cached, nil
		}
	}

	cmd := exec.Command("gpg", "--decrypt", "--quiet", "--batch", p.Path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := out.String()
	if p.Storage != nil {
		p.Storage.SetCached(p.Path, result)
	}

	return result, nil
}

func (p *StoredItem) Raw() string {
	file, err := os.Open(p.Path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func (p *StoredItem) Metadata() string {
	decrypted, err := p.decrypt()
	if err != nil {
		return ""
	}

	lines := strings.SplitN(decrypted, "\n", 2)
	if len(lines) < 2 {
		return ""
	}

	return strings.TrimSpace(lines[1])
}

func (p *StoredItem) Password() string {
	decrypted, err := p.decrypt()
	if err != nil {
		return ""
	}

	lines := strings.SplitN(decrypted, "\n", 2)
	if len(lines) == 0 {
		return ""
	}

	return strings.TrimSpace(lines[0])
}
