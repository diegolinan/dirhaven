package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodeSelection[T any](t *testing.T, raw string) T {
	t.Helper()
	var result T
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("invalid JSON selection: %v", err)
	}
	if len(raw) > maxSelectionOutputBytes {
		t.Fatalf("oversized response %d", len(raw))
	}
	return result
}

func testSelectionFile(t *testing.T) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "unicode.txt")
	content := "intro\r\nMCP first\nsegundo MCP\r\nfinal mcp"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	return service, path, content
}

func TestReadFileLinesExactPagingAndRevision(t *testing.T) {
	s, path, original := testSelectionFile(t)
	p1Text, err := s.ReadFileLines(path, 1, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	p1 := decodeSelection[lineSelection](t, p1Text)
	sum := sha256.Sum256([]byte(original))
	if p1.FileSHA256 != hex.EncodeToString(sum[:]) || p1.FileBytes != int64(len(original)) || p1.TotalLines != 4 {
		t.Fatalf("incorrect source snapshot: %+v", p1.sourceInfo)
	}
	if p1.NextLine != 3 || len(p1.Lines) != 2 {
		t.Fatalf("first page: %+v", p1)
	}
	p2Text, err := s.ReadFileLines(path, p1.NextLine, 2, p1.FileSHA256)
	if err != nil {
		t.Fatal(err)
	}
	p2 := decodeSelection[lineSelection](t, p2Text)
	if p2.NextLine != 0 || len(p2.Lines) != 2 {
		t.Fatalf("last page: %+v", p2)
	}
	var got strings.Builder
	for _, line := range append(p1.Lines, p2.Lines...) {
		got.WriteString(line.Text)
		switch line.EOL {
		case "lf":
			got.WriteByte('\n')
		case "crlf":
			got.WriteString("\r\n")
		case "none":
		default:
			t.Fatalf("invalid EOL %q", line.EOL)
		}
	}
	if got.String() != original {
		t.Fatalf("round trip changed bytes: %q", got.String())
	}
	if _, err := s.ReadFileLines(path, 0, 2, ""); err == nil {
		t.Fatal("accepted line zero")
	}
	if _, err := s.ReadFileLines(path, 1, maxLinesPerPage+1, ""); err == nil {
		t.Fatal("accepted excessive page size")
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(original, "first", "after")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFileLines(path, 3, 2, p1.FileSHA256); err == nil || !strings.Contains(err.Error(), "revision mismatch") {
		t.Fatalf("accepted changed source: %v", err)
	}
}

func TestSearchTextFileLiteralCaseAndCompleteness(t *testing.T) {
	s, path, _ := testSelectionFile(t)
	a, err := s.SearchTextFile(path, "mcp", 1, 1, false, "")
	if err != nil {
		t.Fatal(err)
	}
	first := decodeSelection[searchSelection](t, a)
	if first.TotalMatches != 3 || len(first.Matches) != 1 || first.Matches[0].Number != 2 || first.NextLine != 3 {
		t.Fatalf("page one: %+v", first)
	}
	secondText, err := s.SearchTextFile(path, "mcp", first.NextLine, 1, false, first.FileSHA256)
	if err != nil {
		t.Fatal(err)
	}
	second := decodeSelection[searchSelection](t, secondText)
	if second.TotalMatches != 3 || second.Matches[0].Number != 3 || second.NextLine != 4 {
		t.Fatalf("page two: %+v", second)
	}
	lastText, err := s.SearchTextFile(path, "mcp", second.NextLine, 1, false, second.FileSHA256)
	if err != nil {
		t.Fatal(err)
	}
	last := decodeSelection[searchSelection](t, lastText)
	if len(last.Matches) != 1 || last.Matches[0].Number != 4 || last.NextLine != 0 {
		t.Fatalf("last page: %+v", last)
	}
	strictText, err := s.SearchTextFile(path, "MCP", 1, 10, true, "")
	if err != nil {
		t.Fatal(err)
	}
	strict := decodeSelection[searchSelection](t, strictText)
	if strict.TotalMatches != 2 || !reflect.DeepEqual([]int{strict.Matches[0].Number, strict.Matches[1].Number}, []int{2, 3}) {
		t.Fatalf("case-sensitive results: %+v", strict)
	}
	missing, err := s.SearchTextFile(path, "unknown", 1, 10, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if page := decodeSelection[searchSelection](t, missing); page.TotalMatches != 0 || page.NextLine != 0 {
		t.Fatal("invented search results")
	}
	if _, err := s.SearchTextFile(path, "", 1, 1, false, ""); err == nil {
		t.Fatal("accepted empty query")
	}
}

func TestSelectionBoundariesAndConfinement(t *testing.T) {
	s, path, _ := testSelectionFile(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SearchTextFile(outside, "secret", 1, 10, false, ""); err == nil {
		t.Fatal("escaped allowed root")
	}
	if _, err := s.ReadFileLines(outside, 1, 10, ""); err == nil {
		t.Fatal("outside file readable")
	}
	if err := os.WriteFile(path, []byte("a\x00b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFileLines(path, 1, 10, ""); err == nil {
		t.Fatal("accepted binary content")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxPhysicalLineBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFileLines(path, 1, 1, ""); err == nil {
		t.Fatal("accepted oversized source line")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxSelectionOutputBytes+100)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFileLines(path, 1, 1, ""); err == nil {
		t.Fatal("published incomplete oversized single line")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSelectedFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := s.SearchTextFile(path, "a", 1, 2, false, ""); err == nil {
		t.Fatal("accepted oversized file")
	}
}

func TestSelectionOutputCapDoesNotLosePages(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.txt")
	var original strings.Builder
	for i := 0; i < 240; i++ {
		fmt.Fprintf(&original, "MATCH-%03d %s\n", i, strings.Repeat("w", 235))
	}
	if err := os.WriteFile(path, []byte(original.String()), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	start, revision := 1, ""
	var collected []sourceLine
	for i := 0; i < 20; i++ {
		raw, err := s.ReadFileLines(path, start, 300, revision)
		if err != nil {
			t.Fatal(err)
		}
		page := decodeSelection[lineSelection](t, raw)
		if revision == "" {
			revision = page.FileSHA256
		}
		collected = append(collected, page.Lines...)
		if page.NextLine == 0 {
			break
		}
		if page.NextLine <= start {
			t.Fatalf("pagination failed to progress: %d", start)
		}
		start = page.NextLine
	}
	if len(collected) != 240 {
		t.Fatalf("lost lines while paging: %d", len(collected))
	}
	var reconstructed strings.Builder
	for _, line := range collected {
		reconstructed.WriteString(line.Text)
		reconstructed.WriteByte('\n')
	}
	if reconstructed.String() != original.String() {
		t.Fatal("pagination changed source")
	}

	start, gotMatches := 1, 0
	for i := 0; i < 20; i++ {
		raw, err := s.SearchTextFile(path, "MATCH-", start, 100, true, revision)
		if err != nil {
			t.Fatal(err)
		}
		page := decodeSelection[searchSelection](t, raw)
		if page.TotalMatches != 240 {
			t.Fatalf("incorrect complete match count: %d", page.TotalMatches)
		}
		gotMatches += len(page.Matches)
		if page.NextLine == 0 {
			break
		}
		if page.NextLine <= start {
			t.Fatalf("search pagination failed to progress: %d", start)
		}
		start = page.NextLine
	}
	if gotMatches != 240 {
		t.Fatalf("lost paginated matches: %d", gotMatches)
	}
}

func TestSelectionCannotReadThroughOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "private.txt")
	if err := os.WriteFile(secret, []byte("SECRET-OUTSIDE-ROOT"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(secret, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privileges unavailable: %v", err)
		}
		t.Fatal(err)
	}
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFileLines(link, 1, 1, ""); err == nil {
		t.Fatal("read_file_lines followed symlink outside allowed root")
	}
	if _, err := s.SearchTextFile(link, "SECRET", 1, 1, false, ""); err == nil {
		t.Fatal("search_text_file followed symlink outside allowed root")
	}
}

func TestSelectionHonorsRequestCancellation(t *testing.T) {
	s, path, _ := testSelectionFile(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.ReadFileLinesContext(ctx, path, 1, 2, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read continued: %v", err)
	}
	if _, err := s.SearchTextFileContext(ctx, path, "MCP", 1, 2, false, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search continued: %v", err)
	}
}

func TestSelectionMCPToolsDoNotDuplicateResult(t *testing.T) {
	s, path, _ := testSelectionFile(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := mcp.NewInMemoryTransports()
	session, err := s.MCP().Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "selection-test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	list, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	if len(list.Tools) != 17 || !names["read_file_lines"] || !names["search_text_file"] || !names["search_text_files"] {
		t.Fatalf("invalid tool discovery: %d %+v", len(list.Tools), names)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"read_file_lines", map[string]any{"path": path, "start_line": 1, "limit": 2}},
		{"search_text_file", map[string]any{"path": path, "query": "MCP", "max_matches": 1}},
	} {
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v %+v", tc.name, err, result)
		}
		if result.StructuredContent != nil {
			t.Fatalf("%s duplicated response in structured content", tc.name)
		}
		content := result.Content[0].(*mcp.TextContent).Text
		if !json.Valid([]byte(content)) || len(content) > maxSelectionOutputBytes {
			t.Fatalf("%s invalid or unbounded output", tc.name)
		}
	}
}
