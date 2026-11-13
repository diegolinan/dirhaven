package filesystem

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerBatchSelectionTool adds one Go-only staging capability. Unlike
// existing text search it supports multiple files, OR queries, exact adjacent
// lines and reverse source order in one bounded tool roundtrip.
func (s *Service) registerBatchSelectionTool(server *mcp.Server) {
	noOpenWorld := false
	server.AddTool(&mcp.Tool{
		Name:        "search_text_files",
		Description: "Search 1-8 explicit authorized UTF-8 files for up to 6 literal terms (OR semantics), include up to 3 exact adjacent source lines around every matched line, and return file SHA-256, total/first/last matching lines per file. Use direction=reverse to inspect LATER occurrences and possible corrections first; source order is NOT proof of factual chronology. Output <=16 KiB and paginated with next_cursor, expected_snapshot_sha256. Follow every cursor until done if exhaustive coverage is required. No summaries, inference or regex.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &noOpenWorld},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paths": map[string]any{"type": "array", "minItems": 1, "maxItems": maxBatchFiles,
					"items": map[string]any{"type": "string"}},
				"queries": map[string]any{"type": "array", "minItems": 1, "maxItems": maxBatchQueries,
					"items": map[string]any{"type": "string"}},
				"context_lines":  map[string]any{"type": "integer", "minimum": 0, "maximum": maxBatchContextLines},
				"max_matches":    map[string]any{"type": "integer", "minimum": 1, "maximum": maxBatchMatches},
				"case_sensitive": map[string]any{"type": "boolean"},
				"direction":      map[string]any{"type": "string", "enum": []string{"forward", "reverse"}},
				"cursor": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_index": map[string]any{"type": "integer", "minimum": 0},
						"line":       map[string]any{"type": "integer", "minimum": 1},
					},
					"required":             []string{"file_index", "line"},
					"additionalProperties": false,
				},
				"expected_snapshot_sha256": map[string]any{"type": "string"},
			},
			"required":             []string{"paths", "queries"},
			"additionalProperties": false,
		},
	}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if request == nil || request.Params == nil {
			return toolResult("", errors.New("tool arguments required")), nil
		}
		var args struct {
			Paths         []string     `json:"paths"`
			Queries       []string     `json:"queries"`
			ContextLines  int          `json:"context_lines"`
			MaxMatches    int          `json:"max_matches"`
			CaseSensitive bool         `json:"case_sensitive"`
			Direction     string       `json:"direction"`
			Cursor        *batchCursor `json:"cursor"`
			Expected      string       `json:"expected_snapshot_sha256"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
			return toolResult("", err), nil
		}
		content, err := s.SearchTextFilesContext(ctx,
			args.Paths, args.Queries, args.ContextLines, args.MaxMatches,
			args.CaseSensitive, args.Direction, args.Cursor, args.Expected)
		if err != nil {
			return toolResult("", err), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}, nil
	})
}
