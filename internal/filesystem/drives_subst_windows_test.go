//go:build windows

package filesystem

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in native acceptance with an ephemeral Windows SUBST drive. Never touches
// an existing drive; t.TempDir owns the only writable test content. Cleanup
// happens even if the assertion fails.
func TestNativeDynamicSubstDriveWithoutMCPRestart(t *testing.T) {
	if os.Getenv("FS_NATIVE_SUBST_TEST") != "1" {
		t.Skip("opt-in; uses a disposable Windows SUBST drive mapping")
	}
	if _, err := exec.LookPath("subst.exe"); err != nil {
		t.Skipf("subst unavailable: %v", err)
	}
	mask, _, _ := getLogicalDrives.Call()
	drive := ""
	for ch := byte('Z'); ch >= byte('M'); ch-- {
		if mask&(1<<uint(ch-'A')) == 0 {
			drive = string(ch) + ":"
			break
		}
	}
	if drive == "" {
		t.Skip("no unused drive letters in Z:M range; never override an existing drive")
	}
	service, err := New([]string{"*"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(service.ListAllowedDirectories(), drive+"\\") {
		t.Fatalf("unused %s already advertised", drive)
	}
	dir := t.TempDir()
	mount := exec.Command("subst.exe", drive, dir)
	if output, err := mount.CombinedOutput(); err != nil {
		t.Fatalf("could not safely mount disposable drive %s: %v (%s)", drive, err, output)
	}
	mounted := true
	defer func() {
		if mounted {
			if output, err := exec.Command("subst.exe", drive, "/D").CombinedOutput(); err != nil {
				t.Errorf("failed to remove owned SUBST drive %s: %v (%s)", drive, err, output)
			}
		}
	}()
	if listing := service.ListAllowedDirectories(); !strings.Contains(listing, drive+"\\") {
		t.Fatalf("mounted %s missing from dynamic roots: %s", drive, listing)
	}
	file := filepath.Join(drive+"\\", "roundtrip.txt")
	if err := service.WriteFile(file, "dynamic-mounted-drive"); err != nil {
		t.Fatalf("could not write via new SUBST drive: %v", err)
	}
	got, err := service.ReadTextFile(file, 0, 0)
	if err != nil || got != "dynamic-mounted-drive" {
		t.Fatalf("wrong data on mapped drive: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "roundtrip.txt")); err != nil {
		t.Fatalf("mapped drive did not resolve to disposable fixture: %v", err)
	}
	if output, err := exec.Command("subst.exe", drive, "/D").CombinedOutput(); err != nil {
		t.Fatalf("could not remove disposable drive %s: %v (%s)", drive, err, output)
	}
	mounted = false
	if listing := service.ListAllowedDirectories(); strings.Contains(listing, drive+"\\") {
		t.Fatalf("unmounted drive still listed: %s", listing)
	}
	if _, err := service.ReadTextFile(file, 0, 0); err == nil {
		t.Fatal("unmounted drive still authorized without restart")
	}
}
