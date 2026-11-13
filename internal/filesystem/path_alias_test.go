package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// An allowed directory may have an OS-level alias spelling. Paths written
// through that alias must remain within the *resolved* allowed root, including
// for files that do not exist yet.
func TestAllowedRootSymlinkAliasAndNewLeaf(t *testing.T) {
	backing := t.TempDir()
	aliases := t.TempDir()
	alias := filepath.Join(aliases, "root-alias")
	if err := os.Symlink(backing, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("creating directory symlinks requires OS permissions: %v", err)
		}
		t.Fatal(err)
	}
	s, err := New([]string{alias})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(alias, "new-file.txt")
	if err := s.WriteFile(file, "inside"); err != nil {
		t.Fatalf("safe alias path rejected: %v", err)
	}
	if got, err := s.ReadTextFile(file, 0, 0); err != nil || got != "inside" {
		t.Fatalf("safe alias read: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(backing, "new-file.txt")); err != nil || string(got) != "inside" {
		t.Fatalf("underlying approved root not updated: %q %v", got, err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(backing, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("creating escape symlink requires OS permissions: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := s.ReadTextFile(filepath.Join(alias, "escape", "secret.txt"), 0, 0); err == nil {
		t.Fatal("directory alias bypassed os.Root escape protection")
	}
	if err := s.WriteFile(filepath.Join(alias, "escape", "created.txt"), "bad"); err == nil {
		t.Fatal("directory alias permitted write outside allowed root")
	}
}
