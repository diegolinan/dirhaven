package setupwizard

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTunnelFingerprintOnlyOnExplicitPath(t *testing.T) {
	dir := t.TempDir()
	name := "tunnel-client"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	content := []byte("test-content-not-a-real-executable")
	if err := os.WriteFile(path, content, 0700); err != nil {
		t.Fatal(err)
	}
	sha, size, err := fingerprintTunnelFile(path)
	want := sha256.Sum256(content)
	if err != nil || sha != hex.EncodeToString(want[:]) || size != int64(len(content)) {
		t.Fatalf("unexpected fingerprint: %q %d %v", sha, size, err)
	}
	for _, invalid := range []string{"", path + ".missing", dir, "relative", filepath.Join(dir, "different")} {
		if _, _, err := fingerprintTunnelFile(invalid); err == nil {
			t.Errorf("accepted invalid path %q", invalid)
		}
	}
}
func TestTunnelFingerprintEndpointGuards(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("q", 64)}
	for _, tc := range []struct {
		method, token, origin string
		status                int
	}{
		{http.MethodGet, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "", "", http.StatusUnauthorized},
		{http.MethodPost, "bad", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "https://evil.example", http.StatusForbidden},
		{http.MethodPost, w.token, "http://" + w.address, http.StatusOK},
	} {
		r := requestWizard(t, w, tc.method, "/api/tunnel-fingerprint", tc.token, tc.origin, tunnelFingerprintRequest{Path: "relative"})
		if r.Code != tc.status {
			t.Fatalf("got %d, want %d", r.Code, tc.status)
		}
		if r.Code == http.StatusOK && !strings.Contains(r.Body.String(), `"authenticity":"not_verified"`) {
			t.Fatalf("authenticity overstated: %s", r.Body.String())
		}
	}
}
