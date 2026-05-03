package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupGPGBeforeOverwrite_copiesToTildeSuffix(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "t.gpg")
	if err := os.WriteFile(entry, []byte("payload-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Storage{path: dir, cache: map[string]string{}}

	if err := s.BackupGPGBeforeOverwrite(entry); err != nil {
		t.Fatalf("backup: %v", err)
	}
	backup := entry + "~"
	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload-a" {
		t.Fatalf("backup content = %q want payload-a", got)
	}

	if err := os.WriteFile(entry, []byte("payload-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.BackupGPGBeforeOverwrite(entry); err != nil {
		t.Fatalf("second backup: %v", err)
	}
	got, err = os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload-b" {
		t.Fatalf("after second backup, ~ file = %q want payload-b (latest copy of entry)", got)
	}
}

func TestBackupGPGBeforeOverwrite_noOpWhenMissing(t *testing.T) {
	dir := t.TempDir()
	s := &Storage{path: dir, cache: map[string]string{}}
	missing := filepath.Join(dir, "nope.gpg")
	if err := s.BackupGPGBeforeOverwrite(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing + "~"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup file, stat err=%v", err)
	}
}

func TestIndexAll_skipsTildeBackups(t *testing.T) {
	dir := t.TempDir()
	x := filepath.Join(dir, "x.gpg")
	if err := os.WriteFile(x, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(x+"~", []byte("tilde"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Storage{path: dir, cache: map[string]string{}}
	s.IndexAll()
	if len(s.passwords) != 1 {
		t.Fatalf("len(passwords)=%d want 1", len(s.passwords))
	}
	if s.passwords[0].Path != x {
		t.Fatalf("indexed path = %q want %q", s.passwords[0].Path, x)
	}
}

func TestBackupGPGBeforeOverwrite_rejectsOutsideStore(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(t.TempDir(), "outside.gpg")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Storage{path: dir, cache: map[string]string{}}
	if err := s.BackupGPGBeforeOverwrite(other); err == nil {
		t.Fatal("expected error for path outside store")
	}
}
