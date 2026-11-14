package setupwizard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPickerAuthorizationAndNoImplicitScope(t *testing.T) {
	var calls atomic.Int32
	const selectedPath = "/example/user-selected"
	w := wizard{
		address:    "127.0.0.1:35620",
		token:      strings.Repeat("p", 64),
		command:    "dirhaven",
		pickerBusy: &atomic.Bool{},
		pickDirectory: func(context.Context) (string, error) {
			calls.Add(1)
			return selectedPath, nil
		},
	}
	if got := requestWizard(t, w, http.MethodGet, "/api/pick-directory", w.token, "", nil); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET unexpectedly allowed to launch a picker: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/pick-directory", "", "", nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("Missing token was accepted: %d", got.Code)
	}
	if got := requestWizard(t, w, http.MethodPost, "/api/pick-directory", w.token, "https://evil.example", nil); got.Code != http.StatusForbidden {
		t.Fatalf("Cross-origin picker unexpectedly launched: %d", got.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized request launched OS picker")
	}
	good := requestWizard(t, w, http.MethodPost, "/api/pick-directory", w.token, "http://"+w.address, nil)
	if good.Code != http.StatusOK {
		t.Fatalf("picker failed: %d", good.Code)
	}
	var response struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(good.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Path != selectedPath || calls.Load() != 1 {
		t.Fatalf("incorrect picker output: %+v", response)
	}
	// Selecting a path doesn't grant filesystem access to it.
	if scope := w.prepareScope(scopeRequest{Mode: "scoped"}); scope.OK {
		t.Fatal("picker granted scope without a separate validation request")
	}
	w.pickerBusy.Store(true)
	if got := requestWizard(t, w, http.MethodPost, "/api/pick-directory", w.token, "", nil); got.Code != http.StatusConflict {
		t.Fatalf("parallel OS picker allowed: %d", got.Code)
	}
}

func TestPickerCancellationAndUnavailableDialog(t *testing.T) {
	w := wizard{
		address: "127.0.0.1:35620",
		token:   strings.Repeat("p", 64),
		pickDirectory: func(context.Context) (string, error) {
			return "", errPickerCancelled
		},
	}
	resp := requestWizard(t, w, http.MethodPost, "/api/pick-directory", w.token, "", nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancelled dialog not reported clearly: %d, %s", resp.Code, resp.Body.String())
	}
	w.pickDirectory = func(context.Context) (string, error) {
		return "", errors.New("no UI available")
	}
	resp = requestWizard(t, w, http.MethodPost, "/api/pick-directory", w.token, "", nil)
	if resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "no UI available") || !strings.Contains(resp.Body.String(), "paste") {
		t.Fatalf("dialog failure not handled safely: %d, %s", resp.Code, resp.Body.String())
	}
}
