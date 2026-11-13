package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A fake drive table tests mounted/unmounted roots without altering a real disk.
func TestDynamicDriveAuthorizationChangesWithoutRestart(t *testing.T) {
	root := t.TempDir()
	drive1 := filepath.Join(root, "drive-one")
	drive2 := filepath.Join(root, "drive-two")
	foreign := filepath.Join(root, "not-mounted")
	for _, dir := range []string{drive1, drive2, foreign} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	available := []string{drive1}
	s := &Service{discoverRoots: func() ([]string, error) {
		return append([]string(nil), available...), nil
	}}
	f1, f2 := filepath.Join(drive1, "one.txt"), filepath.Join(drive2, "two.txt")
	if err := s.WriteFile(f1, "one"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(f2, "two"); err == nil {
		t.Fatal("new drive was available before being mounted")
	}
	if _, err := s.ReadTextFile(filepath.Join(foreign, "no.txt"), 0, 0); err == nil {
		t.Fatal("access outside authorized mount was permitted")
	}
	available = []string{drive1, drive2}
	if err := s.WriteFile(f2, "two"); err != nil {
		t.Fatalf("new mount denied without restart: %v", err)
	}
	if listing := s.ListAllowedDirectories(); !strings.Contains(listing, drive2) {
		t.Fatalf("new drive missing in discovery: %s", listing)
	}
	available = []string{drive2}
	if _, err := s.ReadTextFile(f1, 0, 0); err == nil {
		t.Fatal("removed drive still accessible")
	}
	if err := s.WriteFile(filepath.Join(drive1, "new.txt"), "should fail"); err == nil {
		t.Fatal("removed drive could still be written")
	}
	if contents, err := s.ReadTextFile(f2, 0, 0); err != nil || contents != "two" {
		t.Fatalf("still-mounted drive inaccessible: %q %v", contents, err)
	}
	if listing := s.ListAllowedDirectories(); strings.Contains(listing, drive1) || !strings.Contains(listing, drive2) {
		t.Fatalf("stale allowed directories: %s", listing)
	}
	if _, err := os.Stat(filepath.Join(drive1, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unmounted drive was modified: %v", err)
	}
}

func TestDynamicDriveDiscoveryFailureFailsClosed(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "protected.txt")
	if err := os.WriteFile(file, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	fail := false
	s := &Service{roots: []string{base}, discoverRoots: func() ([]string, error) {
		if fail {
			return nil, errors.New("mock drive enumeration failure")
		}
		return nil, nil
	}}
	if _, err := s.ReadTextFile(file, 0, 0); err != nil {
		t.Fatal(err)
	}
	fail = true
	if _, err := s.listAllowedDirectoriesLive(); err == nil {
		t.Fatal("discovery failure presented as an allowed directory list")
	}
	if _, err := s.ReadTextFile(file, 0, 0); err == nil {
		t.Fatal("discovery failure accepted a prior root")
	}
	if err := s.WriteFile(filepath.Join(base, "new.txt"), "bad"); err == nil {
		t.Fatal("discovery failure authorized a mutation")
	}
	if _, err := os.Stat(filepath.Join(base, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fail-closed write created a file: %v", err)
	}
}

func TestDynamicDriveMCPRefreshesWithoutReinitialization(t *testing.T) {
	base := t.TempDir()
	alpha, beta := filepath.Join(base, "alpha"), filepath.Join(base, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	mounted := []string{alpha}
	s := &Service{discoverRoots: func() ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), mounted...), nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "dynamic-drive-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(tool string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	list := call("list_allowed_directories", map[string]any{})
	if list.IsError || !strings.Contains(list.Content[0].(*mcp.TextContent).Text, alpha) ||
		strings.Contains(list.Content[0].(*mcp.TextContent).Text, beta) {
		t.Fatal("initial MCP roots incorrect:", list.Content)
	}
	mu.Lock()
	mounted = []string{alpha, beta}
	mu.Unlock()
	list = call("list_allowed_directories", map[string]any{})
	if list.IsError || !strings.Contains(list.Content[0].(*mcp.TextContent).Text, beta) {
		t.Fatal("MCP did not discover mounted volume:", list.Content)
	}
	dest := filepath.Join(beta, "mounted.txt")
	write := call("write_file", map[string]any{"path": dest, "content": "mount-refresh"})
	if write.IsError {
		t.Fatal("MCP could not write to new mount:", write.Content)
	}
	read := call("read_text_file", map[string]any{"path": dest})
	if read.IsError || read.Content[0].(*mcp.TextContent).Text != "mount-refresh" {
		t.Fatal("MCP could not read from new mount:", read.Content)
	}
	mu.Lock()
	mounted = []string{alpha}
	mu.Unlock()
	list = call("list_allowed_directories", map[string]any{})
	if list.IsError || strings.Contains(list.Content[0].(*mcp.TextContent).Text, beta) {
		t.Fatal("removed drive appeared in MCP listing:", list.Content)
	}
	if r := call("read_text_file", map[string]any{"path": dest}); !r.IsError {
		t.Fatal("MCP permitted read after simulated drive removal")
	}
}

func TestDriveWildcardWindowsOnlyAndActualVolumeInventory(t *testing.T) {
	s, err := New([]string{"*"})
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatal("accepted Windows wildcard on a non-Windows platform")
		}
		return
	}
	if err != nil {
		t.Fatal("Windows all-drives wildcard rejected:", err)
	}
	roots, err := s.availableRoots()
	if err != nil || len(roots) == 0 {
		t.Fatalf("no usable Windows drives discovered: %v %v", roots, err)
	}
	for _, root := range roots {
		if len(root) != 3 || root[1:] != ":\\" {
			t.Fatalf("wildcard returned a non drive-letter root %q", root)
		}
		fi, err := os.Stat(root)
		if err != nil || !fi.IsDir() {
			t.Fatalf("unavailable drive advertised: %q %v", root, err)
		}
	}
	if _, err := s.ReadTextFile("\\\\remote\\share\\file.txt", 0, 0); err == nil {
		t.Fatal("unbounded UNC path was accepted by drive wildcard")
	}
}
