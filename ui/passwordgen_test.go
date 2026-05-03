package ui

import (
	"strings"
	"testing"
)

func isDefaultStrongDashBodyRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func TestGeneratePassword_Strong(t *testing.T) {
	t.Parallel()
	for range 200 {
		s, err := GeneratePassword(PasswordGenStrong)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != defaultStrongLength {
			t.Fatalf("length %d, want %d", len(s), defaultStrongLength)
		}
		for _, r := range s {
			if !strings.ContainsRune(defaultStrongCharset, r) {
				t.Fatalf("char %q not in charset", r)
			}
		}
	}
}

func TestGeneratePassword_SpaceSeparated(t *testing.T) {
	t.Parallel()
	for range 200 {
		s, err := GeneratePassword(PasswordGenSpaceSeparated)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, " ") {
			t.Fatalf("expected space in %q", s)
		}
		compact := strings.ReplaceAll(s, " ", "")
		if len(compact) != defaultStrongLength {
			t.Fatalf("compact length %d, want %d", len(compact), defaultStrongLength)
		}
		for _, r := range compact {
			if !strings.ContainsRune(defaultStrongCharset, r) {
				t.Fatalf("char %q not in charset", r)
			}
		}
	}
}

func TestGeneratePassword_DashSeparated(t *testing.T) {
	t.Parallel()
	for range 200 {
		s, err := GeneratePassword(PasswordGenDashSeparated)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, "-") {
			t.Fatalf("expected dash in %q", s)
		}
		compact := strings.ReplaceAll(s, "-", "")
		if len(compact) != defaultStrongLength {
			t.Fatalf("compact length %d, want %d", len(compact), defaultStrongLength)
		}
		for _, r := range compact {
			if !isDefaultStrongDashBodyRune(r) {
				t.Fatalf("char %q not in dash-body charset", r)
			}
		}
	}
}

func TestGeneratePassword_Lite(t *testing.T) {
	t.Parallel()
	for range 200 {
		s, err := GeneratePassword(PasswordGenLite)
		if err != nil {
			t.Fatal(err)
		}
		n := len(s)
		if n < 7 || n > 9 {
			t.Fatalf("length %d, want 7..9", n)
		}
		for _, r := range s {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				t.Fatalf("char %q not alphanumeric", r)
			}
		}
	}
}
