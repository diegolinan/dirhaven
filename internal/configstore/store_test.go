package configstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadUpdateRevoke(t *testing.T) {
	root := t.TempDir()
	store := WithPath(filepath.Join(root, "nested", "scope.json"))
	_, rev, exists, err := store.Load()
	if err != nil || exists || rev != "" {
		t.Fatalf("unexpected initial state: %q %v %v", rev, exists, err)
	}
	a := Scope{Version: SchemaVersion, Mode: "scoped", Roots: []string{root}}
	revisionA, err := store.Save(a, "")
	if err != nil || revisionA == "" {
		t.Fatalf("save failed: %v", err)
	}
	loaded, loadedRevision, exists, err := store.Load()
	if err != nil || !exists || loadedRevision != revisionA || loaded.Roots[0] != root {
		t.Fatalf("roundtrip failed: %+v %s %v %v", loaded, loadedRevision, exists, err)
	}
	if _, err := store.Save(Scope{Version: SchemaVersion, Mode: "all"}, ""); err == nil {
		t.Fatal("stale client overwrote saved policy")
	}
	revisionB, err := store.Save(Scope{Version: SchemaVersion, Mode: "all"}, revisionA)
	if err != nil || revisionB == revisionA {
		t.Fatalf("update failed: %s %v", revisionB, err)
	}
	if err := store.Remove(revisionA); err == nil {
		t.Fatal("stale client revoked configuration")
	}
	if err := store.Remove(revisionB); err != nil {
		t.Fatal(err)
	}
	_, _, exists, err = store.Load()
	if err != nil || exists {
		t.Fatalf("policy still exists after explicit revoke: %v %v", exists, err)
	}
}

func TestInvalidSavedStateIsNotOverwritten(t *testing.T) {
	d := t.TempDir()
	s := WithPath(filepath.Join(d, "scope.json"))
	if err := os.WriteFile(s.Path, []byte(`{"version":1,"mode":"all","roots":["/"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Load(); err == nil {
		t.Fatal("invalid all-access configuration loaded")
	}
	if _, err := s.Save(Scope{Version: SchemaVersion, Mode: "scoped", Roots: []string{d}}, ""); err == nil {
		t.Fatal("malformed saved state silently overwritten")
	}
	data, err := os.ReadFile(s.Path)
	if err != nil || !strings.Contains(string(data), `"roots":["/"]`) {
		t.Fatal("malformed state was modified")
	}
}

func TestRejectSymlinksAndInvalidRoots(t *testing.T) {
	d := t.TempDir()
	s := WithPath(filepath.Join(d, "scope.json"))
	target := filepath.Join(d, "target.json")
	if err := os.WriteFile(target, []byte("something"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, s.Path); err == nil {
		if _, _, _, err := s.Load(); err == nil {
			t.Fatal("followed saved config symlink")
		}
		if _, err := s.Save(Scope{Version: SchemaVersion, Mode: "scoped", Roots: []string{d}}, ""); err == nil {
			t.Fatal("overwrote symlink")
		}
	}
	for _, bad := range []Scope{
		{Version: SchemaVersion, Mode: "scoped"},
		{Version: SchemaVersion, Mode: "scoped", Roots: []string{"relative"}},
		{Version: SchemaVersion, Mode: "scoped", Roots: []string{d, d}},
		{Version: SchemaVersion, Mode: "all", Roots: []string{d}},
		{Version: 999, Mode: "all"},
	} {
		if err := Validate(bad); err == nil {
			t.Errorf("accepted bad policy: %+v", bad)
		}
	}
}
