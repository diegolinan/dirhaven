package filesystem

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFilesystemOperationsParitySlice(t *testing.T) {
	root := t.TempDir()
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "nested")
	if err := s.CreateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDirectory(filepath.Join(dir, "empty")); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := s.WriteFile(a, "one\n  two\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(b, "three\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(a, "one\n  two\n"); err != nil {
		t.Fatal("overwrite failed:", err)
	}
	if _, err := s.ReadMultipleFiles(nil); err == nil {
		t.Fatal("accepted empty paths")
	}
	multiple, err := s.ReadMultipleFiles([]string{a, filepath.Join(root, "missing")})
	if err != nil || !strings.Contains(multiple, "one\n") || !strings.Contains(multiple, "Error -") {
		t.Fatalf("multiple %q %v", multiple, err)
	}
	before, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := s.EditFile(a, []Edit{{OldText: "two", NewText: "changed"}}, true)
	if err != nil || !strings.Contains(diff, "+  changed") {
		t.Fatalf("dry run diff %q %v", diff, err)
	}
	after, err := os.ReadFile(a)
	if err != nil || string(after) != string(before) {
		t.Fatal("dry run changed file")
	}
	if _, err := s.EditFile(a, []Edit{{OldText: "two", NewText: "changed"}}, false); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(a)
	if err != nil || !strings.Contains(string(after), "  changed") {
		t.Fatalf("edit %q %v", after, err)
	}
	if _, err := s.EditFile(a, []Edit{{OldText: "missing text", NewText: "x"}}, false); err == nil {
		t.Fatal("missing edit should fail")
	}
	tree, err := s.Tree(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Children *[]any `json:"children"`
	}
	if err := json.Unmarshal([]byte(tree), &nodes); err != nil {
		t.Fatal(err)
	}
	hasEmpty := false
	for _, n := range nodes {
		if n.Name == "empty" {
			hasEmpty = true
			if n.Children == nil {
				t.Fatalf("empty folder missing children: %s", tree)
			}
		}
	}
	if !hasEmpty {
		t.Fatal("empty directory not listed")
	}
	search, err := s.SearchFiles(dir, "**/*.txt", nil)
	if err != nil || !strings.Contains(search, a) || !strings.Contains(search, b) {
		t.Fatalf("search %q %v", search, err)
	}
	filtered, err := s.SearchFiles(dir, "**/*.txt", []string{"b.txt"})
	if err != nil || strings.Contains(filtered, b) || !strings.Contains(filtered, a) {
		t.Fatalf("exclude %q %v", filtered, err)
	}
	sizes, err := s.ListDirectoryWithSizes(dir, "size")
	if err != nil || !strings.Contains(sizes, "Total: 2 files, 1 directories") {
		t.Fatalf("size %q %v", sizes, err)
	}
	info, err := s.GetFileInfo(a)
	if err != nil || !strings.Contains(info, "isFile: true") {
		t.Fatalf("info %q %v", info, err)
	}
	if err := s.MoveFile(a, b); err == nil {
		t.Fatal("move overwrote existing file")
	}
	c := filepath.Join(dir, "c.txt")
	if err := s.MoveFile(a, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("move left source file")
	}
	if _, err := os.Stat(c); err != nil {
		t.Fatal("move destination missing", err)
	}
}

func TestFilesystemMutationsOutsideRootsDenied(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDirectory(filepath.Join(other, "bad")); err == nil {
		t.Fatal("outside mkdir permitted")
	}
	if err := s.WriteFile(filepath.Join(other, "bad.txt"), "test"); err == nil {
		t.Fatal("outside write permitted")
	}
	if err := s.WriteFile(filepath.Join(root, "..", filepath.Base(other), "bad.txt"), "test"); err == nil {
		t.Fatal("traversal write permitted")
	}
	src := filepath.Join(root, "source")
	if err := s.WriteFile(src, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveFile(src, filepath.Join(other, "target")); err == nil {
		t.Fatal("outside move permitted")
	}
	if err := os.Symlink(other, filepath.Join(root, "escape")); err == nil {
		if err := s.WriteFile(filepath.Join(root, "escape", "bad.txt"), "test"); err == nil {
			t.Fatal("symlink escape permitted")
		}
	}
	if _, err := os.Stat(filepath.Join(other, "bad.txt")); err == nil {
		t.Fatal("wrote outside")
	}
	s2, err := New([]string{root, other})
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.MoveFile(src, filepath.Join(other, "target")); err == nil {
		t.Fatal("cross-root move is not implemented and must fail closed")
	}
}

func TestMCPDiscoveryAndCalls(t *testing.T) {
	root := t.TempDir()
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "filesystem-test", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	lst, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	for _, name := range []string{"read_file", "read_text_file", "read_media_file", "read_multiple_files", "write_file", "edit_file", "create_directory", "list_directory", "list_directory_with_sizes", "directory_tree", "move_file", "search_files", "get_file_info", "list_allowed_directories", "read_file_lines", "search_text_file", "search_text_files"} {
		expected[name] = true
	}
	if len(lst.Tools) != len(expected) {
		t.Fatalf("expected %d tools got %d", len(expected), len(lst.Tools))
	}
	for _, tool := range lst.Tools {
		if !expected[tool.Name] {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		delete(expected, tool.Name)
	}
	if len(expected) != 0 {
		t.Fatalf("missing tools: %v", expected)
	}
	call := func(name string, args any) *mcp.CallToolResult {
		t.Helper()
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("MCP call %s error: %v", name, err)
		}
		if r.IsError {
			t.Fatalf("MCP call %s tool error: %+v", name, r.Content)
		}
		return r
	}
	dir := filepath.Join(root, "hello")
	call("create_directory", map[string]any{"path": dir})
	file := filepath.Join(dir, "text.txt")
	call("write_file", map[string]any{"path": file, "content": "hi\nthere\n"})
	read := call("read_text_file", map[string]any{"path": file})
	content, ok := read.StructuredContent.(map[string]any)
	if !ok || content["content"] != "hi\nthere\n" {
		t.Fatalf("bad structured result: %#v", read.StructuredContent)
	}
	call("read_file", map[string]any{"path": file})
	call("list_allowed_directories", map[string]any{})
	call("list_directory", map[string]any{"path": root})
	call("list_directory_with_sizes", map[string]any{"path": dir})
	call("directory_tree", map[string]any{"path": root})
	call("search_files", map[string]any{"path": root, "pattern": "**/*.txt"})
	call("read_multiple_files", map[string]any{"paths": []string{file}})
	call("get_file_info", map[string]any{"path": file})
	call("edit_file", map[string]any{"path": file, "edits": []map[string]string{{"oldText": "there", "newText": "world"}}, "dryRun": true})
	path2 := filepath.Join(dir, "renamed.txt")
	call("move_file", map[string]any{"source": file, "destination": path2})
	// SDK media content stores original bytes and encodes them to Base64
	// during JSON-RPC serialization. Pre-encoding Data corrupts the payload.
	pngFile := filepath.Join(dir, "image.png")
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pngFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	media := call("read_media_file", map[string]any{"path": pngFile})
	if len(media.Content) != 1 {
		t.Fatalf("expected one media block, got %d", len(media.Content))
	}
	image, isImage := media.Content[0].(*mcp.ImageContent)
	if !isImage {
		t.Fatalf("expected MCP image block, got %T", media.Content[0])
	}
	if string(image.Data) != string(data) {
		t.Fatalf("image bytes altered by MCP roundtrip: got=%d original=%d", len(image.Data), len(data))
	}
	wavFile := filepath.Join(dir, "silent.wav")
	wav := []byte{'R', 'I', 'F', 'F', 36, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0, 64, 31, 0, 0,
		128, 62, 0, 0, 2, 0, 16, 0, 'd', 'a', 't', 'a', 0, 0, 0, 0}
	if err := os.WriteFile(wavFile, wav, 0600); err != nil {
		t.Fatal(err)
	}
	audioResult := call("read_media_file", map[string]any{"path": wavFile})
	if len(audioResult.Content) != 1 {
		t.Fatalf("expected one audio block, got %d", len(audioResult.Content))
	}
	audio, isAudio := audioResult.Content[0].(*mcp.AudioContent)
	if !isAudio {
		t.Fatalf("expected MCP audio block, got %T", audioResult.Content[0])
	}
	if string(audio.Data) != string(wav) {
		t.Fatalf("audio bytes altered by MCP roundtrip: got=%d original=%d", len(audio.Data), len(wav))
	}
}
