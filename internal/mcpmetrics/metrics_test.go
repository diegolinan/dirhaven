package mcpmetrics

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMiddlewareOnlyEmitsSafeMetadata(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "calls.jsonl")
	recorder, err := Open(logFile)
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "metrics-test", Version: "1"}, nil)
	server.AddReceivingMiddleware(recorder.Middleware())
	server.AddTool(&mcp.Tool{
		Name:        "read_text_file",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"secret": map[string]any{"type": "string"}}},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if strings.Contains(string(req.Params.Arguments), "ERROR") {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "NOLOG-ERROR-RESPONSE"}}}, nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "NOLOG-OUTPUT-SECRET"}},
			StructuredContent: map[string]any{"payload": "NOLOG-DUPLICATE-SECRET"},
		}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "metrics-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, secret := range []string{"NOLOG-INPUT-SECRET", "ERROR-NOLOG-INPUT"} {
		if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "read_text_file", Arguments: map[string]any{"secret": secret}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "NOLOG") || strings.Contains(string(raw), "ERROR-NOLOG") {
		t.Fatalf("private data leaked into metrics: %q", string(raw))
	}
	scan := bufio.NewScanner(strings.NewReader(string(raw)))
	var events []Event
	for scan.Scan() {
		var e Event
		if err := json.Unmarshal(scan.Bytes(), &e); err != nil {
			t.Fatalf("invalid metrics JSONL: %v", err)
		}
		events = append(events, e)
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two tool calls, got %d: %s", len(events), raw)
	}
	for _, e := range events {
		if e.SchemaVersion != 1 || e.Tool != "read_text_file" || e.ArgumentsBytes <= 0 ||
			e.ResultJSONBytes <= 0 || e.DurationMS < 0 || e.AtUTC == "" {
			t.Fatalf("invalid tool metadata: %+v", e)
		}
	}
	if events[0].IsError || !events[1].IsError {
		t.Fatalf("tool error flags wrong: %+v", events)
	}
}

func TestClientControlledToolNamesCannotLeak(t *testing.T) {
	if got := publicToolName("C:\\\\Users\\\\private\\\\secret.txt"); got != "unrecognized" {
		t.Fatalf("unsafe tool name was logged as %q", got)
	}
	if got := publicToolName("sk-SECRET-CREDENTIAL"); got != "unrecognized" {
		t.Fatalf("credential-like tool name was logged as %q", got)
	}
	if got := publicToolName("search_text_file"); got != "search_text_file" {
		t.Fatalf("registered tool name lost: %q", got)
	}
}

func TestOpenRejectsUnsafeLocations(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("empty path accepted")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing", "a.jsonl")); err == nil {
		t.Fatal("missing parent accepted")
	}
}
