// Package filesystem implements Dirhaven's local, root-scoped filesystem MCP service.
// All path operations are constrained to explicit roots, including symlink resolution.
package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegolinan/dirhaven/internal/mcpmetrics"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Service keeps explicitly approved roots separate from dynamic drive discovery.
// Dynamic roots are resolved on each call so inserting/removing a drive never
// requires restarting MCP or reinitializing the client.
type Service struct {
	roots         []string
	discoverRoots func() ([]string, error)
	metrics       *mcpmetrics.Recorder
}

// SetMetrics injects optional metadata-only telemetry before MCP startup.
func (s *Service) SetMetrics(metrics *mcpmetrics.Recorder) { s.metrics = metrics }

func New(allowed []string) (*Service, error) {
	if len(allowed) == 0 {
		return nil, errors.New("at least one allowed root required")
	}
	seen := map[string]bool{}
	roots := make([]string, 0, len(allowed))
	allDrives := false
	for _, root := range allowed {
		if root == "" {
			return nil, errors.New("empty allowed root")
		}
		if root == "*" {
			allDrives = true
			continue
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		actual, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("root %q: %w", root, err)
		}
		fi, err := os.Stat(actual)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("root %q is not a directory", root)
		}
		actual = filepath.Clean(actual)
		if !seen[actual] {
			roots = append(roots, actual)
			seen[actual] = true
		}
	}
	sort.Strings(roots)
	s := &Service{roots: roots}
	if allDrives {
		if !dynamicDriveDiscoverySupported() {
			return nil, errors.New("wildcard '*' for dynamically mounted drive letters is supported only on Windows")
		}
		s.discoverRoots = discoverDriveRoots
		if _, err := s.availableRoots(); err != nil {
			return nil, fmt.Errorf("enumerate Windows drives: %w", err)
		}
	}
	return s, nil
}

// availableRoots is an instantaneous allowlist. A drive unavailable at this
// moment is not authorized; a newly mounted drive may be authorized immediately.
func (s *Service) availableRoots() ([]string, error) {
	combined := append([]string(nil), s.roots...)
	if s.discoverRoots != nil {
		drives, err := s.discoverRoots()
		if err != nil {
			// No cached or previous allowlist is trusted when discovery fails.
			return nil, fmt.Errorf("drive discovery failed: %w", err)
		}
		combined = append(combined, drives...)
	}
	seen := make(map[string]bool, len(combined))
	roots := make([]string, 0, len(combined))
	for _, root := range combined {
		if root == "" || !filepath.IsAbs(root) {
			return nil, errors.New("drive discovery returned a non-absolute root")
		}
		root = filepath.Clean(root)
		key := strings.ToLower(root)
		if !seen[key] {
			roots = append(roots, root)
			seen[key] = true
		}
	}
	sort.Strings(roots)
	return roots, nil
}

// Roots is the currently accessible snapshot, retained for code compatibility.
// The MCP list_allowed_directories tool uses availableRoots directly to report
// discovery errors instead of presenting a misleading empty allowlist.
func (s *Service) Roots() []string {
	roots, _ := s.availableRoots()
	return roots
}

func (s *Service) ListAllowedDirectories() string {
	roots, _ := s.availableRoots()
	return "Allowed directories:\n" + strings.Join(roots, "\n")
}

func (s *Service) listAllowedDirectoriesLive() (string, error) {
	roots, err := s.availableRoots()
	if err != nil {
		return "", err
	}
	return "Allowed directories:\n" + strings.Join(roots, "\n"), nil
}

func (s *Service) ListDirectory(input string) (string, error) {
	entries, err := s.readDir(input)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		kind := "[FILE]"
		if entry.IsDir() {
			kind = "[DIR]"
		}
		lines = append(lines, kind+" "+entry.Name())
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Service) ReadTextFile(input string, head, tail int) (string, error) {
	if head < 0 || tail < 0 {
		return "", errors.New("head and tail must be nonnegative")
	}
	if head > 0 && tail > 0 {
		return "", errors.New("head and tail cannot both be set")
	}
	root, rel, err := s.openPath(input)
	if err != nil {
		return "", err
	}
	defer root.Close()
	fi, err := root.Stat(rel)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%q is a directory", input)
	}
	b, err := root.ReadFile(rel)
	if err != nil {
		return "", err
	}
	contents := string(b)
	if head == 0 && tail == 0 {
		return contents, nil
	}
	// Match Node's read_text_file/headFile/tailFile observable semantics.
	// Neither mode includes a separator after a selected line; tail selects
	// the final empty line when the source ends with '\n'.
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	if head > 0 {
		if len(lines) > head {
			lines = lines[:head]
		}
	} else if tail > 0 {
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Service) MCP() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "dirhaven", Version: "0.1.0-dev"}, nil)
	if s.metrics != nil {
		server.AddReceivingMiddleware(s.metrics.Middleware())
	}
	openWorld := false
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &openWorld}
	add := func(name, desc string, props map[string]any, required []string, action func(map[string]json.RawMessage) (string, error)) {
		if props == nil {
			props = map[string]any{}
		}
		schema := map[string]any{"type": "object", "properties": props, "$schema": "http://json-schema.org/draft-07/schema#"}
		if len(required) > 0 {
			schema["required"] = required
		}
		server.AddTool(&mcp.Tool{Name: name, Description: desc, InputSchema: schema, OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}}, "required": []string{"content"}}, Annotations: readOnly}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]json.RawMessage{}
			if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
				if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
					return toolResult("", err), nil
				}
			}
			out, err := action(args)
			return toolResult(out, err), nil
		})
	}
	add("list_allowed_directories", "Returns the directories this server can access.", nil, nil, func(_ map[string]json.RawMessage) (string, error) { return s.listAllowedDirectoriesLive() })
	pathProp := map[string]any{"path": map[string]any{"type": "string"}}
	readPath := func(args map[string]json.RawMessage) (string, error) {
		var path string
		if err := json.Unmarshal(args["path"], &path); err != nil || path == "" {
			return "", errors.New("path required")
		}
		return path, nil
	}
	add("list_directory", "List files and directories under an allowed path.", pathProp, []string{"path"}, func(args map[string]json.RawMessage) (string, error) {
		path, err := readPath(args)
		if err != nil {
			return "", err
		}
		return s.ListDirectory(path)
	})
	add("read_text_file", "Read a text file inside an allowed root, optionally taking head or tail lines.", map[string]any{
		"path": pathProp["path"],
		"head": map[string]any{"type": "number", "description": "If provided, returns only the first N lines of the file"},
		"tail": map[string]any{"type": "number", "description": "If provided, returns only the last N lines of the file"},
	}, []string{"path"}, func(args map[string]json.RawMessage) (string, error) {
		path, err := readPath(args)
		if err != nil {
			return "", err
		}
		var head, tail int
		if b, ok := args["head"]; ok {
			if err := json.Unmarshal(b, &head); err != nil {
				return "", err
			}
		}
		if b, ok := args["tail"]; ok {
			if err := json.Unmarshal(b, &tail); err != nil {
				return "", err
			}
		}
		return s.ReadTextFile(path, head, tail)
	})
	s.registerExtra(server)
	s.registerSelectionTools(server)
	s.registerBatchSelectionTool(server)
	return server
}

func toolResult(text string, err error) *mcp.CallToolResult {
	r := &mcp.CallToolResult{}
	if err != nil {
		r.IsError = true
		text = err.Error()
	}
	r.Content = []mcp.Content{&mcp.TextContent{Text: text}}
	if err == nil {
		r.StructuredContent = map[string]any{"content": text}
	}
	return r
}

// The stdio Serve implementation lives in stdio_compat.go so repeated
// filesystem MCP initialization can match the existing Node service.
