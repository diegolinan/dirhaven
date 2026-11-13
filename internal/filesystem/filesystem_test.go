package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFilesystemRootsAndTraversal(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(allowed, "sample.txt"), []byte("first\nsecond\nthird\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New([]string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.ListAllowedDirectories(), allowed) {
		t.Fatal("missing root")
	}
	if content, err := s.ReadTextFile(filepath.Join(allowed, "sample.txt"), 2, 0); err != nil || content != "first\nsecond" {
		t.Fatalf("head %q %v", content, err)
	}
	if content, err := s.ReadTextFile(filepath.Join(allowed, "sample.txt"), 0, 2); err != nil || content != "third\n" {
		t.Fatalf("tail %q %v", content, err)
	}
	if _, err := s.ReadTextFile(filepath.Join(outside, "secret.txt"), 0, 0); err == nil {
		t.Fatal("outside file allowed")
	}
	if _, err := s.ReadTextFile(filepath.Join(allowed, "..", filepath.Base(outside), "secret.txt"), 0, 0); err == nil {
		t.Fatal("traversal allowed")
	}
}
func TestSymlinkEscapes(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	s, err := New([]string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadTextFile(filepath.Join(link, "secret"), 0, 0); err == nil {
		t.Fatal("symlink escape allowed")
	}
}
func TestPathRequiredAndErrors(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("accepted missing roots")
	}
	s, err := New([]string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadTextFile("", 0, 0); err == nil {
		t.Fatal("missing path accepted")
	}
	if _, err := s.ReadTextFile(s.Roots()[0], 2, 2); err == nil {
		t.Fatal("mixed head/tail accepted")
	}
	if _, err := s.ListDirectory(filepath.Join(s.Roots()[0], "absent")); err == nil {
		t.Fatal("missing dir accepted")
	}
}
