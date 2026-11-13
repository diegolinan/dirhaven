package filesystem

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Selection tools extend the original Node-compatible 14 tools. The existing
// tool signatures and outputs are untouched. New tools return one bounded
// JSON text block, without a duplicate StructuredContent copy.
func (s *Service) registerSelectionTools(server *mcp.Server) {
	falseValue := false
	annotation := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &falseValue}
	str := func() map[string]any { return map[string]any{"type": "string"} }
	positive := func() map[string]any { return map[string]any{"type": "integer", "minimum": 1} }
	register := func(name, description string, props map[string]any, required []string, fn func(context.Context, json.RawMessage) (string, error)) {
		server.AddTool(&mcp.Tool{
			Name: name, Description: description,
			Annotations: annotation,
			InputSchema: map[string]any{
				"type": "object", "properties": props, "required": required,
				"additionalProperties": false,
			},
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if request == nil || request.Params == nil {
				return toolResult("", errors.New("tool arguments required")), nil
			}
			content, err := fn(ctx, request.Params.Arguments)
			if err != nil {
				return toolResult("", err), nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}, nil
		})
	}
	register("read_file_lines",
		"Read a bounded, exact range of numbered UTF-8 text lines. Returns file SHA-256, total lines, EOL type and next_line for paging. Re-send file_sha256 as expected_sha256 on subsequent pages to detect edits. Output is never silently truncated. File must be <= 32 MiB.",
		map[string]any{
			"path": str(), "start_line": positive(), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxLinesPerPage},
			"expected_sha256": str(),
		}, []string{"path"}, func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				Path      string `json:"path"`
				StartLine *int   `json:"start_line"`
				Limit     int    `json:"limit"`
				Expected  string `json:"expected_sha256"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			start := 1
			if args.StartLine != nil {
				start = *args.StartLine
			}
			return s.ReadFileLinesContext(ctx, args.Path, start, args.Limit, args.Expected)
		},
	)
	register("search_text_file",
		"Search a single UTF-8 file for a literal string, returning matching complete lines with source line numbers, count of ALL matching lines, file SHA-256 and next_line for paging. Default case-insensitive. Pair with search_files for filename discovery and read_file_lines for context; no regex, semantic compression or inference. File must be <= 32 MiB.",
		map[string]any{
			"path": str(), "query": str(), "start_line": positive(),
			"max_matches":     map[string]any{"type": "integer", "minimum": 1, "maximum": maxSearchMatches},
			"case_sensitive":  map[string]any{"type": "boolean"},
			"expected_sha256": str(),
		}, []string{"path", "query"}, func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				Path          string `json:"path"`
				Query         string `json:"query"`
				StartLine     *int   `json:"start_line"`
				MaxMatches    int    `json:"max_matches"`
				CaseSensitive bool   `json:"case_sensitive"`
				Expected      string `json:"expected_sha256"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			start := 1
			if args.StartLine != nil {
				start = *args.StartLine
			}
			return s.SearchTextFileContext(ctx, args.Path, args.Query, start, args.MaxMatches, args.CaseSensitive, args.Expected)
		},
	)
}
