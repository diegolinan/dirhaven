package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStandaloneStdioFilesystemRoundTrip exercises the independently built
// Dirhaven executable over a real stdio MCP process, not an in-memory server.
// Its only file operations target disposable test directories. This is NOT a
// substitute for a ChatGPT Web -> Secure MCP Tunnel -> Dirhaven acceptance test.
func TestStandaloneStdioFilesystemRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	executable := filepath.Join(t.TempDir(), "dirhaven-test")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", executable, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build standalone Dirhaven CLI: %v: %s", err, output)
	}

	authorized := filepath.Join(t.TempDir(), "authorized")
	if err := os.Mkdir(authorized, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	untouched := filepath.Join(outside, "untouched.txt")
	if err := os.WriteFile(untouched, []byte("outside remains untouched"), 0600); err != nil {
		t.Fatal(err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "dirhaven-stdio-acceptance", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command: exec.Command(executable, "serve", "--allow", authorized),
	}, nil)
	if err != nil {
		t.Fatalf("connect to real Dirhaven stdio process: %v", err)
	}
	defer session.Close()

	discovered, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("discover MCP tools: %v", err)
	}
	toolNames := make(map[string]bool, len(discovered.Tools))
	for _, tool := range discovered.Tools {
		toolNames[tool.Name] = true
	}
	if len(toolNames) != 17 {
		t.Fatalf("expected 17 distinct MCP tools, got %d: %v", len(toolNames), toolNames)
	}
	for _, name := range []string{
		"list_allowed_directories", "read_text_file", "write_file",
		"edit_file", "read_file_lines", "search_text_file", "search_text_files",
	} {
		if !toolNames[name] {
			t.Fatalf("standalone executable did not advertise %q", name)
		}
	}

	call := func(name string, arguments map[string]any, wantError bool) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: name, Arguments: arguments,
		})
		if err != nil {
			t.Fatalf("%s transport failure: %v", name, err)
		}
		if result == nil {
			t.Fatalf("%s returned nil response", name)
		}
		if result.IsError != wantError {
			t.Fatalf("%s IsError=%t want %t: %#v", name, result.IsError, wantError, result.Content)
		}
		return result
	}

	call("list_allowed_directories", map[string]any{}, false)

	inRoot := filepath.Join(authorized, "chatgpt-acceptance.txt")
	call("write_file", map[string]any{"path": inRoot, "content": "before\n"}, false)
	call("edit_file", map[string]any{
		"path":  inRoot,
		"edits": []map[string]string{{"oldText": "before", "newText": "after"}},
	}, false)
	after, err := os.ReadFile(inRoot)
	if err != nil || string(after) != "after\n" {
		t.Fatalf("real executable did not edit authorized file: %q %v", after, err)
	}
	read := call("read_text_file", map[string]any{"path": inRoot}, false)
	data, ok := read.StructuredContent.(map[string]any)
	if !ok || data["content"] != "after\n" {
		t.Fatalf("wrong content from stdio MCP read: %#v", read.StructuredContent)
	}
	call("read_file_lines", map[string]any{"path": inRoot}, false)
	call("search_text_file", map[string]any{"path": inRoot, "query": "after"}, false)
	call("search_text_files", map[string]any{"paths": []string{inRoot}, "queries": []string{"after"}}, false)

	call("read_text_file", map[string]any{"path": untouched}, true)
	call("write_file", map[string]any{"path": untouched, "content": "unsafe"}, true)
	call("write_file", map[string]any{"path": filepath.Join(outside, "created.txt"), "content": "unsafe"}, true)
	if got, err := os.ReadFile(untouched); err != nil || string(got) != "outside remains untouched" {
		t.Fatalf("outside file changed after denied MCP calls: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file created despite denied MCP call: %v", err)
	}

	t.Logf("real stdio binary passed: %d tools, authorized edit, read, source selection, and rejected out-of-root operations", len(toolNames))
}
