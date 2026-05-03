package ui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
)

// PasswordGenKind selects how GeneratePassword builds a password.
type PasswordGenKind int

const (
	PasswordGenSpaceSeparated PasswordGenKind = iota
	PasswordGenDashSeparated
	PasswordGenStrong
	PasswordGenLite
)

const (
	defaultStrongCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	// Raw body for dash-separated passwords must not contain '-', so delimiter bytes are unambiguous.
	defaultStrongCharsetDashRaw = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	defaultStrongLength         = 16
	liteCharset                 = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// GeneratePassword returns a password for the given mode using crypto/rand.
func GeneratePassword(mode PasswordGenKind) (string, error) {
	switch mode {
	case PasswordGenStrong:
		return generateDefaultStrong()
	case PasswordGenSpaceSeparated:
		raw, err := generateDefaultStrong()
		if err != nil {
			return "", err
		}
		return formatWithChunkSeparator(raw, " ")
	case PasswordGenDashSeparated:
		raw, err := randomFromCharset(defaultStrongCharsetDashRaw, defaultStrongLength)
		if err != nil {
			return "", err
		}
		return formatWithChunkSeparator(raw, "-")
	case PasswordGenLite:
		return generateLite()
	default:
		return "", fmt.Errorf("unknown password gen kind: %d", mode)
	}
}

// generateDefaultStrong is the former default generator: 16 chars from defaultStrongCharset.
func generateDefaultStrong() (string, error) {
	return randomFromCharset(defaultStrongCharset, defaultStrongLength)
}

func randomFromCharset(charset string, length int) (string, error) {
	password := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	n := len(charset)
	for i := 0; i < length; i++ {
		password[i] = charset[int(randomBytes[i])%n]
	}
	return string(password), nil
}

// generateLite uses only letters and digits; length is uniformly random in [7, 9].
func generateLite() (string, error) {
	var lenByte [1]byte
	if _, err := rand.Read(lenByte[:]); err != nil {
		return "", err
	}
	length := 7 + int(lenByte[0])%3
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	cs := liteCharset
	n := len(cs)
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = cs[int(randomBytes[i])%n]
	}
	return string(out), nil
}

// randomChunkLayoutFor16 returns chunk sizes in {3,4} summing to 16 (only valid partitions).
func randomChunkLayoutFor16() ([]int, error) {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	if b[0]%2 == 0 {
		return []int{4, 4, 4, 4}, nil
	}
	sizes := []int{3, 3, 3, 3, 4}
	for i := len(sizes) - 1; i > 0; i-- {
		var bb [1]byte
		if _, err := rand.Read(bb[:]); err != nil {
			return nil, err
		}
		j := int(bb[0]) % (i + 1)
		sizes[i], sizes[j] = sizes[j], sizes[i]
	}
	return sizes, nil
}

func formatWithChunkSeparator(raw, sep string) (string, error) {
	if len(raw) != defaultStrongLength {
		return "", errors.New("unexpected raw password length")
	}
	sizes, err := randomChunkLayoutFor16()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	pos := 0
	for i, sz := range sizes {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(raw[pos : pos+sz])
		pos += sz
	}
	return b.String(), nil
}
