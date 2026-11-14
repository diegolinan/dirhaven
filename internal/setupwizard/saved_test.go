package setupwizard

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegolinan/dirhaven/internal/configstore"
)

func TestSavedConfigAPIExplicitAuthorizationAndRevoke(t *testing.T) {
	dir := t.TempDir()
	config := configstore.WithPath(filepath.Join(t.TempDir(), "dirhaven", "scope.json"))
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("s", 64), command: filepath.Join(dir, "dirhaven"), store: &config}

	req := saveRequest{Mode: "scoped", Paths: []string{dir}}
	if response := requestWizard(t, w, http.MethodGet, "/api/saved", "", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("saved configuration leaked without token: %d", response.Code)
	}
	if response := requestWizard(t, w, http.MethodPost, "/api/save", "", "", req); response.Code != http.StatusUnauthorized {
		t.Fatalf("save without token accepted: %d", response.Code)
	}
	if response := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "https://evil.example", req); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin save accepted: %d", response.Code)
	}
	empty := requestWizard(t, w, http.MethodGet, "/api/saved", w.token, "", nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"exists":false`) {
		t.Fatalf("unexpected empty saved config: %d %s", empty.Code, empty.Body.String())
	}
	for _, invalid := range []saveRequest{
		{Mode: "all"},
		{Mode: "scoped", Paths: []string{"relative"}},
	} {
		if got := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "", invalid); got.Code != http.StatusUnprocessableEntity {
			t.Fatalf("unsafe preferences accepted: %+v / %d", invalid, got.Code)
		}
	}
	if _, err := os.Stat(config.Path); !os.IsNotExist(err) {
		t.Fatal("invalid or unauthenticated scope unexpectedly wrote to disk")
	}
	first := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "", req)
	if first.Code != http.StatusOK {
		t.Fatalf("save failed: %d %s", first.Code, first.Body.String())
	}
	var saved savedResponse
	if err := json.Unmarshal(first.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.OK || saved.Revision == "" || len(saved.Roots) != 1 {
		t.Fatalf("bad saved response: %+v", saved)
	}
	revision := saved.Revision
	if got := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "", req); got.Code != http.StatusConflict {
		t.Fatalf("stale browser overwrite accepted: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/revoke", w.token, "", revokeRequest{}); got.Code != http.StatusConflict {
		t.Fatalf("revoke without a revision accepted: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/revoke", w.token, "", revokeRequest{ExpectedRevision: "stale"}); got.Code != http.StatusConflict {
		t.Fatalf("stale revoke accepted: %d", got.Code)
	}
	updated := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "", saveRequest{
		Mode: "all", AcknowledgeBroadAccess: true, ExpectedRevision: revision,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("explicit broad access save failed: %d %s", updated.Code, updated.Body.String())
	}
	saved = savedResponse{}
	if err := json.Unmarshal(updated.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Mode != "all" || len(saved.Roots) != 0 {
		t.Fatalf("invalid broad saved scope: %+v", saved)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/revoke", w.token, "", revokeRequest{ExpectedRevision: saved.Revision}); got.Code != http.StatusOK {
		t.Fatalf("revoke failed: %d", got.Code)
	}
	if _, err := os.Stat(config.Path); !os.IsNotExist(err) {
		t.Fatal("revoke did not delete only saved config")
	}
}

func TestSavedConfigCorruptFileCannotBeSilentlyOverwritten(t *testing.T) {
	dir := t.TempDir()
	config := configstore.WithPath(filepath.Join(dir, "scope.json"))
	if err := os.WriteFile(config.Path, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("s", 64), store: &config, command: "dirhaven"}
	if response := requestWizard(t, w, http.MethodGet, "/api/saved", w.token, "", nil); response.Code != http.StatusConflict {
		t.Fatalf("corrupt state not flagged: %d", response.Code)
	}
	if response := requestWizard(t, w, http.MethodPost, "/api/save", w.token, "", saveRequest{
		Mode: "all", AcknowledgeBroadAccess: true,
	}); response.Code != http.StatusConflict {
		t.Fatalf("corrupt state was overwritten: %d", response.Code)
	}
}
