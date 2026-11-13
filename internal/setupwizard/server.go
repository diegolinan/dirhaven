// Package setupwizard provides a local-only, nonpersistent configuration
// assistant. It never starts a tunnel or accesses file contents.
package setupwizard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed web/*
var webAssets embed.FS

const maxRequestBytes = 16 << 10

type scopeRequest struct {
	Mode                   string   `json:"mode"`
	Paths                  []string `json:"paths"`
	AcknowledgeBroadAccess bool     `json:"acknowledgeBroadAccess"`
}

type scopeResponse struct {
	OK            bool     `json:"ok"`
	Error         string   `json:"error,omitempty"`
	Roots         []string `json:"roots,omitempty"`
	Command       string   `json:"command,omitempty"`
	MCPClientJSON string   `json:"mcpClientJSON,omitempty"`
}

type wizard struct {
	address string
	token   string
	command string
}

// Start opens a browser-based wizard on a random IPv4 loopback port.
// It never modifies the installed server, an existing tunnel, or user files.
// A one-time token in the URL fragment authorizes API calls, and the fragment
// is removed from browser history immediately after page startup.
func Start(ctx context.Context, output io.Writer) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("open private setup listener: %w", err)
	}
	defer listener.Close()

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate setup authorization: %w", err)
	}
	token := hex.EncodeToString(secret)
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Dirhaven executable: %w", err)
	}
	w := wizard{
		address: listener.Addr().String(),
		token:   token,
		command: executable,
	}
	server := &http.Server{
		Handler:           w.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	url := "http://" + w.address + "/#" + token
	fmt.Fprintln(output, "Dirhaven Setup Wizard: "+url)
	fmt.Fprintln(output, "This local-only assistant does not request API keys, create tunnels, or modify your files.")
	fmt.Fprintln(output, "Press Ctrl+C to close setup.")
	if err := openBrowser(url); err != nil {
		fmt.Fprintln(output, "Browser did not open automatically. Open the private URL above manually.")
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("close setup: %w", err)
		}
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func openBrowser(url string) error {
	var args []string
	var program string
	switch runtime.GOOS {
	case "windows":
		program, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		program, args = "open", []string{url}
	default:
		program, args = "xdg-open", []string{url}
	}
	return exec.Command(program, args...).Start()
}

func (w wizard) handler() http.Handler {
	mux := http.NewServeMux()
	for _, asset := range []struct {
		route, filename, mime string
	}{
		{"/", "web/index.html", "text/html; charset=utf-8"},
		{"/app.css", "web/app.css", "text/css; charset=utf-8"},
		{"/app.js", "web/app.js", "application/javascript; charset=utf-8"},
		{"/dirhaven-mark.svg", "web/dirhaven-mark.svg", "image/svg+xml"},
	} {
		asset := asset
		mux.HandleFunc(asset.route, func(out http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet || req.URL.Path != asset.route {
				http.NotFound(out, req)
				return
			}
			data, err := webAssets.ReadFile(asset.filename)
			if err != nil {
				http.Error(out, "asset unavailable", http.StatusInternalServerError)
				return
			}
			out.Header().Set("Content-Type", asset.mime)
			out.Write(data)
		})
	}
	mux.HandleFunc("/api/validate", w.handleValidate)

	return http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
		out.Header().Set("Cache-Control", "no-store")
		out.Header().Set("X-Content-Type-Options", "nosniff")
		out.Header().Set("X-Frame-Options", "DENY")
		out.Header().Set("Referrer-Policy", "no-referrer")
		out.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if req.Host != w.address {
			http.Error(out, "host not permitted", http.StatusForbidden)
			return
		}
		if origin := req.Header.Get("Origin"); origin != "" && origin != "http://"+w.address {
			http.Error(out, "origin not permitted", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(out, req)
	})
}

func (w wizard) handleValidate(out http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(out, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Dirhaven-Setup-Token")), []byte(w.token)) != 1 {
		http.Error(out, "setup authorization required", http.StatusUnauthorized)
		return
	}
	if req.Header.Get("Content-Type") != "application/json" {
		http.Error(out, "JSON required", http.StatusUnsupportedMediaType)
		return
	}
	req.Body = http.MaxBytesReader(out, req.Body, maxRequestBytes)
	defer req.Body.Close()
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	var input scopeRequest
	if err := dec.Decode(&input); err != nil {
		http.Error(out, "invalid setup request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(out, "unexpected trailing content", http.StatusBadRequest)
		return
	}
	resp := w.prepareScope(input)
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(out).Encode(resp)
}

func (w wizard) prepareScope(input scopeRequest) scopeResponse {
	var args []string
	var roots []string
	switch input.Mode {
	case "all":
		if !input.AcknowledgeBroadAccess {
			return scopeResponse{Error: "Confirm broad filesystem access explicitly."}
		}
		if len(input.Paths) != 0 {
			return scopeResponse{Error: "Do not mix broad access with explicit roots."}
		}
		args = []string{"serve", "--allow-all"}
	case "scoped":
		if len(input.Paths) == 0 || len(input.Paths) > 24 {
			return scopeResponse{Error: "Choose between one and 24 existing directories."}
		}
		seen := map[string]bool{}
		args = []string{"serve"}
		for _, raw := range input.Paths {
			path := strings.TrimSpace(raw)
			if path == "" || !filepath.IsAbs(path) {
				return scopeResponse{Error: "Every directory must be an absolute path."}
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return scopeResponse{Error: fmt.Sprintf("Cannot access selected directory: %s", path)}
			}
			info, err := os.Stat(resolved)
			if err != nil || !info.IsDir() {
				return scopeResponse{Error: fmt.Sprintf("Not an accessible directory: %s", path)}
			}
			resolved = filepath.Clean(resolved)
			key := resolved
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			roots = append(roots, resolved)
			args = append(args, "--allow", resolved)
		}
		if len(roots) == 0 {
			return scopeResponse{Error: "Choose at least one existing directory."}
		}
	default:
		return scopeResponse{Error: "Choose scoped access or explicitly authorize broad access."}
	}
	cliCommand := append([]string{w.command}, args...)
	parts := make([]string, len(cliCommand))
	for i, arg := range cliCommand {
		parts[i] = quoteForShell(arg)
	}
	command := strings.Join(parts, " ")
	if runtime.GOOS == "windows" {
		command = "& " + command
	}
	client := struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}{
		MCPServers: map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}{"dirhaven": {Command: w.command, Args: args}},
	}
	formatted, err := json.MarshalIndent(client, "", "  ")
	if err != nil {
		return scopeResponse{Error: "Cannot format MCP client settings."}
	}
	return scopeResponse{OK: true, Roots: roots, Command: command, MCPClientJSON: string(formatted)}
}

func quoteForShell(value string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
