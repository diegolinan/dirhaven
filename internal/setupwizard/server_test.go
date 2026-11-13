package setupwizard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func requestWizard(t *testing.T, w wizard, method, route, token, origin string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "http://"+w.address+route, reader)
	req.Host = w.address
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Dirhaven-Setup-Token", token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	w.handler().ServeHTTP(rec, req)
	return rec
}

func TestWizardBoundToHostAndRequiresToken(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("a", 64), command: "/tmp/dirhaven"}
	resp := requestWizard(t, w, http.MethodGet, "/", "", "", nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "DIRHAVEN") {
		t.Fatalf("setup page missing: code=%d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), w.token) {
		t.Fatal("secret leaked into static HTML")
	}
	for _, asset := range []string{"/app.js", "/app.css", "/dirhaven-mark.svg"} {
		r := requestWizard(t, w, http.MethodGet, asset, "", "", nil)
		if r.Code != http.StatusOK || len(r.Body.Bytes()) == 0 {
			t.Fatalf("missing static asset %s: %d", asset, r.Code)
		}
	}
	payload := scopeRequest{Mode: "scoped", Paths: []string{t.TempDir()}}
	if got := requestWizard(t, w, http.MethodPost, "/api/validate", "", "", payload); got.Code != http.StatusUnauthorized {
		t.Fatalf("API without secret: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/validate", "wrong", "", payload); got.Code != http.StatusUnauthorized {
		t.Fatalf("API with wrong secret: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/validate", w.token, "https://attacker.example", payload); got.Code != http.StatusForbidden {
		t.Fatalf("API with hostile Origin: %d", got.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "http://evil.example/", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	w.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("host-header bypass: %d", rec.Code)
	}
}

func TestWizardScopeValidationAndExample(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("b", 64), command: filepath.Join(t.TempDir(), "dirhaven executable")}
	dir := filepath.Join(t.TempDir(), "dir with spaces and 'quote'")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	good := requestWizard(t, w, http.MethodPost, "/api/validate", w.token, "http://"+w.address,
		scopeRequest{Mode: "scoped", Paths: []string{dir, dir}})
	if good.Code != http.StatusOK {
		t.Fatalf("valid path request failed: %d %s", good.Code, good.Body.String())
	}
	var data scopeResponse
	if err := json.Unmarshal(good.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	// macOS resolves temporary /var paths to /private/var, just as
	// filesystem.New does for authorized directory roots.
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !data.OK || len(data.Roots) != 1 || data.Roots[0] != canonical {
		t.Fatalf("incorrect scope result: %+v", data)
	}
	if !strings.Contains(data.Command, "--allow") || !strings.Contains(data.MCPClientJSON, "\"mcpServers\"") {
		t.Fatalf("missing output: %+v", data)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(data.Command, "''quote") {
			t.Fatalf("PowerShell quote not escaped: %q", data.Command)
		}
	} else if !strings.Contains(data.Command, "'\\''quote") {
		t.Fatalf("POSIX quote not escaped: %q", data.Command)
	}
	var example struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(data.MCPClientJSON), &example); err != nil {
		t.Fatal(err)
	}
	if client := example.MCPServers["dirhaven"]; client.Command != w.command || len(client.Args) != 3 || client.Args[2] != canonical {
		t.Fatalf("wrong MCP stdio example: %+v", client)
	}
	for _, bad := range []scopeRequest{
		{Mode: "scoped"},
		{Mode: "scoped", Paths: []string{"relative/file"}},
		{Mode: "scoped", Paths: []string{filepath.Join(t.TempDir(), "not-here")}},
		{Mode: "all"},
		{Mode: "all", AcknowledgeBroadAccess: true, Paths: []string{dir}},
		{Mode: "unknown", Paths: []string{dir}},
	} {
		resp := w.prepareScope(bad)
		if resp.OK || resp.Error == "" {
			t.Errorf("unsafe request was accepted: %+v", bad)
		}
	}
	all := w.prepareScope(scopeRequest{Mode: "all", AcknowledgeBroadAccess: true})
	if !all.OK || !strings.Contains(all.Command, "--allow-all") {
		t.Fatalf("explicit broad-access opt-in rejected: %+v", all)
	}
}
