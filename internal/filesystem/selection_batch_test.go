package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func setupBatchSources(t *testing.T) (*Service, []string, []string) {
	t.Helper()
	dir := t.TempDir()
	contents := []string{
		"intro\r\nfoo initial\ncontext one\nfoo updated\r\noutro\n",
		"header\nbar correction\nfoo final\nend",
	}
	paths := make([]string, len(contents))
	for i, content := range contents {
		paths[i] = filepath.Join(dir, fmt.Sprintf("case%d.txt", i))
		if err := os.WriteFile(paths[i], []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	service, err := New([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return service, paths, contents
}

func callBatch(t *testing.T, s *Service, paths, queries []string, contextLines, limit int, direction string, cursor *batchCursor, snap string) batchSearchPage {
	t.Helper()
	raw, err := s.SearchTextFilesContext(context.Background(), paths, queries, contextLines, limit, false, direction, cursor, snap)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxSelectionOutputBytes {
		t.Fatalf("batch response exceeded budget: %d", len(raw))
	}
	var page batchSearchPage
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestBatchSearchForwardReverseAndExactContext(t *testing.T) {
	s, paths, _ := setupBatchSources(t)
	qs := []string{"foo", "correction"}
	cursor := (*batchCursor)(nil)
	snap := ""
	var matched []string
	var first batchSearchPage
	for iter := 0; iter < 10; iter++ {
		p := callBatch(t, s, paths, qs, 1, 1, "forward", cursor, snap)
		if iter == 0 {
			first = p
			snap = p.SnapshotSHA256
			if p.TotalMatches != 4 || p.Sources[0].TotalMatches != 2 || p.Sources[1].TotalMatches != 2 {
				t.Fatalf("bad totals: %+v", p)
			}
			if p.Sources[0].FirstMatchLine != 2 || p.Sources[0].LastMatchLine != 4 ||
				p.Sources[1].FirstMatchLine != 2 || p.Sources[1].LastMatchLine != 3 {
				t.Fatalf("missing endpoints: %+v", p.Sources)
			}
		} else if p.SnapshotSHA256 != snap {
			t.Fatal("stable files produced different snapshot")
		}
		if len(p.Matches) != 1 {
			t.Fatalf("expected one match per page, got %d", len(p.Matches))
		}
		hit := p.Matches[0]
		matched = append(matched, fmt.Sprintf("%d:%d", hit.FileIndex, hit.MatchLine))
		if hit.MatchLine == 2 && hit.FileIndex == 0 {
			if len(hit.Lines) != 3 || hit.Lines[0].EOL != "crlf" || hit.Lines[1].Text != "foo initial" ||
				hit.Lines[2].Text != "context one" || len(hit.MatchedQueries) != 1 ||
				hit.MatchedQueries[0] != "foo" {
				t.Fatalf("non-exact first match: %+v", hit)
			}
		}
		cursor = p.NextCursor
		if cursor == nil {
			break
		}
	}
	want := []string{"0:2", "0:4", "1:2", "1:3"}
	if fmt.Sprint(matched) != fmt.Sprint(want) {
		t.Fatalf("forward pagination omitted/duplicated matches: %v", matched)
	}
	if first.SnapshotSHA256 == "" {
		t.Fatal("snapshot digest missing")
	}
	matched = nil
	cursor = nil
	snap = ""
	for iter := 0; iter < 10; iter++ {
		p := callBatch(t, s, paths, qs, 0, 2, "reverse", cursor, snap)
		if iter == 0 {
			snap = p.SnapshotSHA256
		}
		for _, hit := range p.Matches {
			matched = append(matched, fmt.Sprintf("%d:%d", hit.FileIndex, hit.MatchLine))
			if len(hit.Lines) != 1 || hit.Lines[0].Number != hit.MatchLine {
				t.Fatalf("context_lines=0 must return original match only: %+v", hit)
			}
		}
		cursor = p.NextCursor
		if cursor == nil {
			break
		}
	}
	want = []string{"1:3", "1:2", "0:4", "0:2"}
	if fmt.Sprint(matched) != fmt.Sprint(want) {
		t.Fatalf("reverse pagination skipped matches: %v", matched)
	}
}

func TestBatchSearchInvalidCursorAndFullSnapshotGuard(t *testing.T) {
	s, paths, contents := setupBatchSources(t)
	q := []string{"foo"}
	page := callBatch(t, s, paths, q, 1, 1, "forward", nil, "")
	if page.NextCursor == nil {
		t.Fatal("expected a continuation")
	}
	fail := func(paths, qs []string, contextLines, limit int, direction string, cursor *batchCursor, snapshot string) {
		t.Helper()
		if _, err := s.SearchTextFilesContext(context.Background(), paths, qs, contextLines, limit, false, direction, cursor, snapshot); err == nil {
			t.Fatal("expected guarded request to fail")
		}
	}
	fail(paths, q, 1, 1, "forward", page.NextCursor, "")
	fail(paths, []string{"foo", "bar"}, 1, 1, "forward", page.NextCursor, page.SnapshotSHA256)
	fail(paths, q, 0, 1, "forward", page.NextCursor, page.SnapshotSHA256)
	fail(paths, q, 1, 1, "reverse", page.NextCursor, page.SnapshotSHA256)
	fail(paths, q, 1, 1, "forward", &batchCursor{FileIndex: 5, Line: 1}, page.SnapshotSHA256)
	if err := os.WriteFile(paths[1], []byte(strings.ReplaceAll(contents[1], "foo final", "foo revised")), 0600); err != nil {
		t.Fatal(err)
	}
	fail(paths, q, 1, 1, "forward", page.NextCursor, page.SnapshotSHA256)
}

func TestBatchSearchNoMatchesIncludesAllSources(t *testing.T) {
	s, paths, _ := setupBatchSources(t)
	p := callBatch(t, s, paths, []string{"nonexistent phrase"}, 2, 2, "reverse", nil, "")
	if p.TotalMatches != 0 || p.NextCursor != nil || len(p.Matches) != 0 || len(p.Sources) != 2 {
		t.Fatalf("incorrect empty search behavior: %+v", p)
	}
	for _, source := range p.Sources {
		if source.FileSHA256 == "" || source.TotalMatches != 0 || source.FirstMatchLine != 0 || source.LastMatchLine != 0 {
			t.Fatalf("missing empty-search provenance: %+v", source)
		}
	}
}

func TestBatchSearchRejectsUnsafeInputs(t *testing.T) {
	s, paths, _ := setupBatchSources(t)
	var scenarios = []struct {
		paths               []string
		queries             []string
		contextLines, limit int
		direction           string
	}{
		{nil, []string{"foo"}, 0, 1, ""},
		{append([]string{}, paths[0], paths[0]), []string{"foo"}, 0, 1, ""},
		{[]string{paths[0]}, nil, 0, 1, ""},
		{[]string{paths[0]}, []string{""}, 0, 1, ""},
		{[]string{paths[0]}, []string{"foo", "FOO"}, 0, 1, ""},
		{[]string{paths[0]}, []string{"foo"}, 4, 1, ""},
		{[]string{paths[0]}, []string{"foo"}, 1, maxBatchMatches + 1, ""},
		{[]string{paths[0]}, []string{"foo"}, 1, 1, "sideways"},
		{[]string{paths[0]}, []string{"\x00"}, 1, 1, ""},
		{[]string{paths[0]}, []string{strings.Repeat("w", 257)}, 1, 1, ""},
	}
	for i, sc := range scenarios {
		if _, err := s.SearchTextFilesContext(context.Background(), sc.paths, sc.queries,
			sc.contextLines, sc.limit, false, sc.direction, nil, ""); err == nil {
			t.Fatalf("scenario %d accepted invalid batch", i)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("foo secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SearchTextFilesContext(context.Background(), []string{paths[0], outside},
		[]string{"foo"}, 1, 10, false, "forward", nil, ""); err == nil {
		t.Fatal("batch leaked eligible source when later file unauthorized")
	}
	cancel, stop := context.WithCancel(context.Background())
	stop()
	if _, err := s.SearchTextFilesContext(cancel, paths, []string{"foo"}, 1, 1,
		false, "forward", nil, ""); err == nil {
		t.Fatal("cancelled scan proceeded")
	}
}

func TestBatchDenseResultCapAndNoSkippedMatches(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "long.txt")
	var b strings.Builder
	for i := 0; i < 135; i++ {
		fmt.Fprintf(&b, "found-%03d %s\n", i, strings.Repeat("x", 230))
	}
	if err := os.WriteFile(f1, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"forward", "reverse"} {
		var cursor *batchCursor
		snap := ""
		var all []int
		for n := 0; n < 70; n++ {
			p := callBatch(t, s, []string{f1}, []string{"found-"}, 2, 40, direction, cursor, snap)
			if n == 0 {
				snap = p.SnapshotSHA256
			}
			if p.TotalMatches != 135 || p.Sources[0].TotalMatches != 135 {
				t.Fatal("incorrect complete match count")
			}
			for _, hit := range p.Matches {
				all = append(all, hit.MatchLine)
				if len(hit.Lines) < 1 || len(hit.Lines) > 5 {
					t.Fatalf("bad context: %+v", hit)
				}
				for _, row := range hit.Lines {
					if row.Number < hit.MatchLine-2 || row.Number > hit.MatchLine+2 {
						t.Fatalf("out-of-range context line: %d vs %d", row.Number, hit.MatchLine)
					}
				}
			}
			cursor = p.NextCursor
			if cursor == nil {
				break
			}
		}
		if len(all) != 135 {
			t.Fatalf("pagination dropped hits: %d", len(all))
		}
		for i, line := range all {
			want := i + 1
			if direction == "reverse" {
				want = 135 - i
			}
			if line != want {
				t.Fatalf("wrong order/dropped hit at %d: got %d want %d", i, line, want)
			}
		}
	}
}

func TestBatchSelectionMCPIntegration(t *testing.T) {
	s, paths, _ := setupBatchSources(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := mcp.NewInMemoryTransports()
	serverSession, err := s.MCP().Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "batch-qa", Version: "1"}, nil)
	cs, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 17 {
		t.Fatalf("unexpected tool count: %d", len(list.Tools))
	}
	response, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_text_files",
		Arguments: map[string]any{
			"paths":         paths,
			"queries":       []string{"foo", "correction"},
			"context_lines": 1,
			"max_matches":   2,
			"direction":     "reverse",
		},
	})
	if err != nil || response.IsError {
		t.Fatalf("MCP batch tool failed %v / %+v", err, response)
	}
	if response.StructuredContent != nil || len(response.Content) != 1 {
		t.Fatal("MCP batch response duplicated or lost contents")
	}
	text := response.Content[0].(*mcp.TextContent).Text
	var page batchSearchPage
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		t.Fatal(err)
	}
	if page.TotalMatches != 4 || len(page.Matches) != 2 || page.Matches[0].FileIndex != 1 || page.Matches[0].MatchLine != 3 || page.NextCursor == nil {
		t.Fatalf("incorrect MCP batch ordering: %+v", page)
	}
}
