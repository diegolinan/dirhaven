// Package mcpmetrics records content-free, opt-in MCP tool call measurements.
// It deliberately avoids paths, argument values, request IDs and response text.
package mcpmetrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A request's tool name is client-controlled. Only names registered by our
// local MCP implementations may enter telemetry; unknown strings are never
// logged verbatim, even if they look like a harmless identifier.
var knownTools = map[string]bool{
	"list_allowed_directories": true, "list_directory": true,
	"read_text_file": true, "read_file": true, "read_multiple_files": true,
	"read_media_file": true, "write_file": true, "edit_file": true,
	"create_directory": true, "list_directory_with_sizes": true,
	"directory_tree": true, "move_file": true, "search_files": true,
	"get_file_info": true, "read_file_lines": true, "search_text_file": true, "search_text_files": true,
}

func publicToolName(name string) string {
	if knownTools[name] {
		return name
	}
	return "unrecognized"
}

type Event struct {
	SchemaVersion   int     `json:"schema_version"`
	AtUTC           string  `json:"at_utc"`
	Tool            string  `json:"tool"`
	ArgumentsBytes  int     `json:"arguments_bytes"`
	ResultJSONBytes int     `json:"result_json_bytes"`
	DurationMS      float64 `json:"duration_ms"`
	IsError         bool    `json:"is_error"`
}

type Recorder struct {
	mu     sync.Mutex
	file   *os.File
	warned bool
}

// Open requires an explicit path and pre-existing parent directory. The
// metrics file is created with private permissions where supported.
func Open(name string) (*Recorder, error) {
	if name == "" {
		return nil, errors.New("metrics path is required")
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(abs); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("metrics path must be a regular file, not a symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	parent, err := os.Stat(filepath.Dir(abs))
	if err != nil || !parent.IsDir() {
		return nil, errors.New("metrics parent directory must already exist")
	}
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	return &Recorder{file: file}, nil
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *Recorder) record(evt Event) {
	raw, err := json.Marshal(evt)
	if err != nil {
		return
	}
	raw = append(raw, '\n')
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	n, err := r.file.Write(raw)
	if err != nil || n != len(raw) {
		if !r.warned {
			// Never put user content, paths, or tool arguments on stderr.
			fmt.Fprintln(os.Stderr, "mcpmetrics: a metrics write failed; MCP responses are unaffected")
			r.warned = true
		}
	}
}

// Middleware sees only tools/call. Its output size is the serialized MCP tool
// RESULT object, not ChatGPT's actual context-token cost or its JSON-RPC wrapper.
func (r *Recorder) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" || req == nil {
				return next(ctx, method, req)
			}
			started := time.Now()
			tool := "unknown"
			inputBytes := 0
			if args, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && args != nil {
				tool = publicToolName(args.Name)
				inputBytes = len(args.Arguments)
			}
			result, err := next(ctx, method, req)
			outputBytes := 0
			failed := err != nil
			if result != nil {
				raw, marshalErr := json.Marshal(result)
				if marshalErr == nil {
					outputBytes = len(raw)
				}
				if call, ok := result.(*mcp.CallToolResult); ok && call.IsError {
					failed = true
				}
			}
			r.record(Event{
				SchemaVersion: 1, AtUTC: started.UTC().Format(time.RFC3339Nano),
				Tool: tool, ArgumentsBytes: inputBytes, ResultJSONBytes: outputBytes,
				DurationMS: float64(time.Since(started).Microseconds()) / 1000,
				IsError:    failed,
			})
			return result, err
		}
	}
}
