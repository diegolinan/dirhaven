package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNodeFilesystemReferenceReadParity(t *testing.T) {
	script := os.Getenv("FS_NODE_REFERENCE")
	if script == "" {
		t.Skip("Set FS_NODE_REFERENCE to a locally installed Node filesystem MCP reference entrypoint")
	}
	if _, err := os.Stat(script); err != nil {
		t.Skip("Node filesystem reference unavailable: " + err.Error())
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node unavailable: " + err.Error())
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte("abc\nxyz\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, b := mcp.NewInMemoryTransports()
	goSS, err := s.MCP().Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer goSS.Close()
	goClient := mcp.NewClient(&mcp.Implementation{Name: "go-reference-test", Version: "1.0"}, nil)
	goSession, err := goClient.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer goSession.Close()
	nodeClient := mcp.NewClient(&mcp.Implementation{Name: "node-reference-test", Version: "1.0"}, nil)
	nodeSession, err := nodeClient.Connect(ctx, &mcp.CommandTransport{Command: exec.Command("node", script, root)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeSession.Close()
	goTools, err := goSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodeTools, err := nodeSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	gmap, nmap := map[string]*mcp.Tool{}, map[string]*mcp.Tool{}
	for _, tool := range goTools.Tools {
		gmap[tool.Name] = tool
	}
	for _, tool := range nodeTools.Tools {
		nmap[tool.Name] = tool
	}
	for name, original := range nmap {
		port, ok := gmap[name]
		if !ok {
			t.Errorf("Go missing tool %s", name)
			continue
		}
		orig, err := json.Marshal(original.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		ported, err := json.Marshal(port.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if string(orig) != string(ported) {
			t.Errorf("INPUT_SCHEMA_DIFF tool=%s node=%s go=%s", name, orig, ported)
		}
	}
	// Existing 14 Node-compatible tools remain unchanged. Three Go-only
	// bounded source-selection tools intentionally extend the interface.
	if len(gmap) != len(nmap)+3 || gmap["read_file_lines"] == nil || gmap["search_text_file"] == nil || gmap["search_text_files"] == nil {
		t.Errorf("tool count go=%d node=%d (expected +3 safe selection tools)", len(gmap), len(nmap))
	}
	type caseInfo struct {
		name string
		args map[string]any
	}
	fixtures := []caseInfo{
		{"list_allowed_directories", map[string]any{}},
		{"list_directory", map[string]any{"path": root}},
		{"read_text_file", map[string]any{"path": filepath.Join(root, "one.txt")}},
		{"read_text_file", map[string]any{"path": filepath.Join(root, "one.txt"), "head": 1}},
		{"read_text_file", map[string]any{"path": filepath.Join(root, "one.txt"), "tail": 1}},
		{"read_file", map[string]any{"path": filepath.Join(root, "one.txt")}},
		{"read_multiple_files", map[string]any{"paths": []string{filepath.Join(root, "one.txt"), filepath.Join(root, "missing.txt")}}},
		{"directory_tree", map[string]any{"path": root}},
		{"search_files", map[string]any{"path": root, "pattern": "**/*.txt"}},
		{"list_directory_with_sizes", map[string]any{"path": root}},
		{"get_file_info", map[string]any{"path": filepath.Join(root, "one.txt")}},
		{"edit_file", map[string]any{"path": filepath.Join(root, "one.txt"), "edits": []map[string]string{{"oldText": "xyz", "newText": "uvw"}}, "dryRun": true}},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			node, err := nodeSession.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatal("node:", err)
			}
			port, err := goSession.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatal("go:", err)
			}
			if node.IsError != port.IsError {
				t.Errorf("error mismatch node=%v go=%v", node.IsError, port.IsError)
			}
			var n, g string
			if len(node.Content) > 0 {
				if c, ok := node.Content[0].(*mcp.TextContent); ok {
					n = c.Text
				}
			}
			if len(port.Content) > 0 {
				if c, ok := port.Content[0].(*mcp.TextContent); ok {
					g = c.Text
				}
			}
			if n != g {
				t.Errorf("RESULT_DIFF tool=%s args=%v\nNODE:\n%s\nGO:\n%s", tc.name, tc.args, trimDiff(n), trimDiff(g))
			}
		})
	}
}
func trimDiff(s string) string {
	if len(s) > 1200 {
		return s[:1200] + "..."
	}
	return strings.ReplaceAll(s, "\r", "\\r")
}
