package filesystem

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Service) registerExtra(server *mcp.Server) {
	isFalse := false
	write := func(idempotent, destructive bool) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: idempotent, DestructiveHint: &destructive, OpenWorldHint: &isFalse}
	}
	readonly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &isFalse}
	str := func() map[string]any { return map[string]any{"type": "string"} }
	property := func(fields ...string) map[string]any {
		props := map[string]any{}
		for _, f := range fields {
			props[f] = str()
		}
		return props
	}
	output := map[string]any{"type": "object", "properties": map[string]any{"content": str()}, "required": []string{"content"}}
	add := func(name, desc string, props map[string]any, req []string, annotation *mcp.ToolAnnotations, handler func(map[string]json.RawMessage) (string, error)) {
		schema := map[string]any{"type": "object", "properties": props, "$schema": "http://json-schema.org/draft-07/schema#"}
		if len(req) > 0 {
			schema["required"] = req
		}
		server.AddTool(&mcp.Tool{Name: name, Description: desc, Annotations: annotation, InputSchema: schema, OutputSchema: output}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a, err := arguments(request)
			if err != nil {
				return toolResult("", err), nil
			}
			value, err := handler(a)
			return toolResult(value, err), nil
		})
	}
	getString := func(a map[string]json.RawMessage, key string) (string, error) {
		var value string
		if err := json.Unmarshal(a[key], &value); err != nil || value == "" {
			return "", fmt.Errorf("%s must be a non-empty string", key)
		}
		return value, nil
	}
	getPath := func(a map[string]json.RawMessage) (string, error) { return getString(a, "path") }
	getExcludes := func(a map[string]json.RawMessage) ([]string, error) {
		var v []string
		if b, ok := a["excludePatterns"]; ok {
			if err := json.Unmarshal(b, &v); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	add("read_file", "Deprecated alias for read_text_file.", map[string]any{"path": str(), "head": map[string]any{"type": "number", "description": "If provided, returns only the first N lines of the file"}, "tail": map[string]any{"type": "number", "description": "If provided, returns only the last N lines of the file"}}, []string{"path"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		var head, tail int
		if b, ok := a["head"]; ok {
			if err = json.Unmarshal(b, &head); err != nil {
				return "", err
			}
		}
		if b, ok := a["tail"]; ok {
			if err = json.Unmarshal(b, &tail); err != nil {
				return "", err
			}
		}
		return s.ReadTextFile(p, head, tail)
	})
	add("read_multiple_files", "Read multiple files, preserving per-file errors and source paths.", map[string]any{"paths": map[string]any{"type": "array", "items": str(), "minItems": 1, "description": "Array of file paths to read. Each path must be a string pointing to a valid file within allowed directories."}}, []string{"paths"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		var p []string
		if err := json.Unmarshal(a["paths"], &p); err != nil {
			return "", err
		}
		return s.ReadMultipleFiles(p)
	})
	add("write_file", "Create or overwrite a file inside an allowed root.", property("path", "content"), []string{"path", "content"}, write(true, true), func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		var content string
		if err := json.Unmarshal(a["content"], &content); err != nil {
			return "", err
		}
		if err := s.WriteFile(p, content); err != nil {
			return "", err
		}
		return "Successfully wrote to " + p, nil
	})
	add("edit_file", "Apply sequential exact text edits and return a unified diff; supports dryRun.", map[string]any{
		"path":   str(),
		"edits":  map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"oldText": map[string]any{"type": "string", "description": "Text to search for - must match exactly"}, "newText": map[string]any{"type": "string", "description": "Text to replace with"}}, "required": []string{"oldText", "newText"}}},
		"dryRun": map[string]any{"type": "boolean", "default": false, "description": "Preview changes using git-style diff format"},
	}, []string{"path", "edits"}, write(false, true), func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		var edits []Edit
		if err := json.Unmarshal(a["edits"], &edits); err != nil {
			return "", err
		}
		var dry bool
		if b, ok := a["dryRun"]; ok {
			if err := json.Unmarshal(b, &dry); err != nil {
				return "", err
			}
		}
		return s.EditFile(p, edits, dry)
	})
	add("create_directory", "Ensure a directory exists, creating necessary parents.", property("path"), []string{"path"}, write(true, false), func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		if err = s.CreateDirectory(p); err != nil {
			return "", err
		}
		return "Successfully created directory " + p, nil
	})
	add("list_directory_with_sizes", "List directory files with formatted sizes and totals.", map[string]any{
		"path":   str(),
		"sortBy": map[string]any{"type": "string", "enum": []string{"name", "size"}, "default": "name", "description": "Sort entries by name or size"},
	}, []string{"path"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		sortBy := "name"
		if b, ok := a["sortBy"]; ok {
			if err = json.Unmarshal(b, &sortBy); err != nil {
				return "", err
			}
		}
		if sortBy != "name" && sortBy != "size" {
			return "", errors.New("sortBy must be name or size")
		}
		return s.ListDirectoryWithSizes(p, sortBy)
	})
	add("directory_tree", "Recursively list directory structure as JSON, respecting excludePatterns.", map[string]any{
		"path":            str(),
		"excludePatterns": map[string]any{"type": "array", "items": str(), "default": []string{}},
	}, []string{"path"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		exclude, err := getExcludes(a)
		if err != nil {
			return "", err
		}
		return s.Tree(p, exclude)
	})
	add("move_file", "Rename or move a file or directory, refusing an existing destination.", property("source", "destination"), []string{"source", "destination"}, write(false, true), func(a map[string]json.RawMessage) (string, error) {
		p, err := getString(a, "source")
		if err != nil {
			return "", err
		}
		dst, err := getString(a, "destination")
		if err != nil {
			return "", err
		}
		if err = s.MoveFile(p, dst); err != nil {
			return "", err
		}
		return "Successfully moved " + p + " to " + dst, nil
	})
	add("search_files", "Recursively match glob paths using ** and excludePatterns.", map[string]any{
		"path": str(), "pattern": str(),
		"excludePatterns": map[string]any{"type": "array", "items": str(), "default": []string{}},
	}, []string{"path", "pattern"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		pattern, err := getString(a, "pattern")
		if err != nil {
			return "", err
		}
		excludes, err := getExcludes(a)
		if err != nil {
			return "", err
		}
		return s.SearchFiles(p, pattern, excludes)
	})
	add("get_file_info", "Return metadata for a file or directory in an allowed root.", property("path"), []string{"path"}, readonly, func(a map[string]json.RawMessage) (string, error) {
		p, err := getPath(a)
		if err != nil {
			return "", err
		}
		return s.GetFileInfo(p)
	})
	// This operation returns native MCP media blocks, not text pretending to be media.
	server.AddTool(&mcp.Tool{
		Name: "read_media_file", Description: "Read image, audio, or other binary file as a base64-encoded MCP content block.",
		Annotations: readonly, InputSchema: map[string]any{"type": "object", "properties": property("path"), "required": []string{"path"}, "$schema": "http://json-schema.org/draft-07/schema#"},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, err := arguments(request)
		if err != nil {
			return toolResult("", err), nil
		}
		p, err := getPath(a)
		if err != nil {
			return toolResult("", err), nil
		}
		content, err := s.ReadBinary(p)
		if err != nil {
			return toolResult("", err), nil
		}
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
		if mt == "" {
			mt = http.DetectContentType(content)
		}
		if idx := strings.IndexByte(mt, ';'); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		encoded := base64.StdEncoding.EncodeToString(content)
		payload := map[string]any{"type": "resource", "resource": map[string]any{"uri": fileURI(p), "mimeType": mt, "blob": encoded}}
		var block mcp.Content
		switch {
		case strings.HasPrefix(mt, "image/"):
			block = &mcp.ImageContent{Data: content, MIMEType: mt}
			payload = map[string]any{"type": "image", "data": encoded, "mimeType": mt}
		case strings.HasPrefix(mt, "audio/"):
			block = &mcp.AudioContent{Data: content, MIMEType: mt}
			payload = map[string]any{"type": "audio", "data": encoded, "mimeType": mt}
		default:
			block = &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: fileURI(p), MIMEType: mt, Blob: content}}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{block}, StructuredContent: map[string]any{"content": []any{payload}}}, nil
	})
}

func arguments(request *mcp.CallToolRequest) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		if err := json.Unmarshal(request.Params.Arguments, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func fileURI(p string) string {
	p = filepath.ToSlash(p)
	if len(p) > 1 && p[1] == ':' {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
