package setupwizard

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTunnelFilePickerRequiresAuthorization(t *testing.T) {
	calls := 0
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("z", 64), pickerBusy: &atomic.Bool{},
		pickTunnelFile: func(context.Context) (string, error) { calls++; return "/some/path/tunnel-client", nil }}
	tests := []struct {
		method, token, origin string
		status                int
	}{
		{http.MethodGet, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "", "", http.StatusUnauthorized},
		{http.MethodPost, "wrong", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "https://invalid.example", http.StatusForbidden},
		{http.MethodPost, w.token, "http://" + w.address, http.StatusOK},
	}
	for _, tc := range tests {
		got := requestWizard(t, w, tc.method, "/api/pick-tunnel-file", tc.token, tc.origin, nil)
		if got.Code != tc.status {
			t.Fatalf("status %d, expected %d", got.Code, tc.status)
		}
	}
	if calls != 1 {
		t.Fatalf("picker invoked %d times", calls)
	}
	w.pickerBusy.Store(true)
	if got := requestWizard(t, w, http.MethodPost, "/api/pick-tunnel-file", w.token, "", nil); got.Code != http.StatusConflict {
		t.Fatalf("concurrent picker allowed: %d", got.Code)
	}
	if calls != 1 {
		t.Fatal("concurrent dialog executed")
	}
}

func TestTunnelFilePickerCancellation(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("z", 64),
		pickTunnelFile: func(context.Context) (string, error) { return "", errPickerCancelled }}
	got := requestWizard(t, w, http.MethodPost, "/api/pick-tunnel-file", w.token, "", nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancellation result: %d %s", got.Code, got.Body.String())
	}
}
