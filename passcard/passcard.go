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

type Password struct {
	Name string
	Path string
}

func (p *Password) decrypt() (string, error) {
	cmd := exec.Command("gpg", "--decrypt", "--quiet", "--batch", p.Path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	
	if err := cmd.Run(); err != nil {
		return "", err
	}
	
	return out.String(), nil
}

func (p *Password) Raw() string {
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

func (p *Password) Metadata() string {
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

func (p *Password) Password() string {
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
