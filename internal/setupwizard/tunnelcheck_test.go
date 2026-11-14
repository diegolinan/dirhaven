package setupwizard

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTunnelDiagnosticPassiveDiscovery(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tunnel-client")
	if err := os.WriteFile(bin, []byte("never execute me"), 0700); err != nil {
		t.Fatal(err)
	}
	discovered := detectTunnelClientWith(func(name string) (string, error) {
		if name != "tunnel-client" {
			t.Errorf("unexpected lookup: %s", name)
		}
		return bin, nil
	}, os.Stat)
	if !discovered.ClientOnPath || discovered.ConnectionStatus != "not_checked" || discovered.ExecutableName != "tunnel-client" {
		t.Fatalf("incorrect passive discovery: %+v", discovered)
	}
	if strings.Contains(discovered.NextStep, dir) {
		t.Fatal("private directory path leaked")
	}
	missing := detectTunnelClientWith(func(string) (string, error) { return "", errors.New("not found") }, os.Stat)
	if missing.ClientOnPath || missing.ConnectionStatus != "not_checked" {
		t.Fatalf("missing client treated as healthy: %+v", missing)
	}
	directory := detectTunnelClientWith(func(string) (string, error) { return dir, nil }, os.Stat)
	if directory.ClientOnPath {
		t.Fatal("directory accepted as client")
	}
}

func TestTunnelDiagnosticRequiresAuthorization(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("d", 64)}
	for _, tc := range []struct {
		method, token, origin string
		want                  int
	}{
		{http.MethodGet, "", "", http.StatusUnauthorized},
		{http.MethodGet, "wrong", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodGet, w.token, "https://evil.example", http.StatusForbidden},
		{http.MethodGet, w.token, "http://" + w.address, http.StatusOK},
	} {
		response := requestWizard(t, w, tc.method, "/api/tunnel-diagnostic", tc.token, tc.origin, nil)
		if response.Code != tc.want {
			t.Fatalf("unexpected status %d vs %d", response.Code, tc.want)
		}
		if tc.want == http.StatusOK && (!strings.Contains(response.Body.String(), `"connectionStatus":"not_checked"`) || strings.Contains(response.Body.String(), dirSensitiveMarker())) {
			t.Fatalf("diagnostic exposed incorrect status or sensitive marker: %s", response.Body.String())
		}
	}
}

func dirSensitiveMarker() string { return "runtime.key" }

func TestChosenTunnelOnlyInspectsMetadata(t *testing.T) {
	dir := t.TempDir()
	name := "tunnel-client"
	if runtime.GOOS == "windows" {
		name = "tunnel-client.exe"
	}
	bin := filepath.Join(dir, name)
	if err := os.WriteFile(bin, []byte("MUST NEVER RUN"), 0700); err != nil {
		t.Fatal(err)
	}
	good := inspectChosenTunnelClient(bin)
	if good.ExecutableName != name || good.ClientOnPath || good.ConnectionStatus != "not_checked" || strings.Contains(good.NextStep, dir) {
		t.Fatalf("unsafe selected file diagnostic: %+v", good)
	}
	for _, path := range []string{"", "relative", filepath.Join(dir, "not-found"), dir, filepath.Join(dir, "not-the-client")} {
		d := inspectChosenTunnelClient(path)
		if d.ExecutableName != "" || d.ConnectionStatus != "not_checked" {
			t.Fatalf("accepted unexpected path %q: %+v", path, d)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(bin, 0600); err != nil {
			t.Fatal(err)
		}
		if d := inspectChosenTunnelClient(bin); d.ExecutableName != "" {
			t.Fatal("accepted nonexecutable file")
		}
	}
}

func TestSelectedTunnelEndpointEnforcesTokenAndOrigin(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("t", 64)}
	for _, tc := range []struct {
		method, token, origin string
		want                  int
	}{
		{http.MethodGet, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "", "", http.StatusUnauthorized},
		{http.MethodPost, "wrong", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "https://evil.example", http.StatusForbidden},
		{http.MethodPost, w.token, "http://" + w.address, http.StatusOK},
	} {
		response := requestWizard(t, w, tc.method, "/api/tunnel-selected", tc.token, tc.origin, selectedTunnelRequest{Path: "relative"})
		if response.Code != tc.want {
			t.Fatalf("status %d not %d", response.Code, tc.want)
		}
		if tc.want == http.StatusOK && !strings.Contains(response.Body.String(), `"connectionStatus":"not_checked"`) {
			t.Fatal("wrong connection state")
		}
	}
}
