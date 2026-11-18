package setupwizard

import (
	"context"
	"encoding/json"
	"github.com/diegolinan/dirhaven/internal/configstore"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test-only browser fixture: real HTTP/validation/persistence against disposable
// folders. Native pickers are deterministic and no tunnel process is launched.
func TestSetupBrowserFixture(t *testing.T) {
	if os.Getenv("DIRHAVEN_UI_FIXTURE") != "1" {
		t.Skip("browser fixture is opt-in")
	}
	root := t.TempDir()
	first := filepath.Join(root, "work & notes")
	second := filepath.Join(root, "archive")
	for _, p := range []string{first, second} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	config := configstore.WithPath(filepath.Join(root, "preferences", "scope.json"))
	listener, err := net.Listen("tcp4", "127.0.0.1:4189")
	if err != nil {
		t.Fatal(err)
	}
	w := wizard{address: listener.Addr().String(), token: strings.Repeat("a", 64), command: filepath.Join(root, "dirhaven"), store: &config, pickerBusy: &atomic.Bool{}, pickDirectory: func(context.Context) (string, error) { return first, nil }, pickTunnelFile: func(context.Context) (string, error) { return filepath.Join(root, "tunnel-client.exe"), nil }}
	done := make(chan struct{})
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	production := w.handler()
	server.Handler = http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
		if req.Host != w.address {
			http.Error(out, "host not permitted", 403)
			return
		}
		switch req.URL.Path {
		case "/__fixture":
			out.Header().Set("Content-Type", "application/json")
			json.NewEncoder(out).Encode(map[string]string{"root": first, "second": second, "token": w.token})
		case "/__fixture/reset":
			if req.Method != "POST" {
				http.Error(out, "method", 405)
				return
			}
			if err := os.Remove(config.Path); err != nil && !os.IsNotExist(err) {
				t.Error(err)
			}
			out.WriteHeader(204)
		case "/__fixture/stop":
			if req.Method != "POST" {
				http.Error(out, "method", 405)
				return
			}
			out.WriteHeader(204)
			go func() { time.Sleep(30 * time.Millisecond); server.Close() }()
		default:
			production.ServeHTTP(out, req)
		}
	})
	go func() { defer close(done); server.Serve(listener) }()
	t.Cleanup(func() { server.Close() })
	t.Log("Dirhaven browser fixture ready at http://" + w.address)
	select {
	case <-done:
	case <-time.After(8 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
