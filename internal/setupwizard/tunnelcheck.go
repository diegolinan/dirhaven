package setupwizard

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// tunnelDiagnostic describes only passive discovery in the current process's PATH.
// It is not a connectivity test: we deliberately do NOT start tunnel-client,
// inspect its profile, read tokens, contact OpenAI, or query running processes.
type tunnelDiagnostic struct {
	Checked          bool   `json:"checked"`
	ClientOnPath     bool   `json:"clientOnPath"`
	ExecutableName   string `json:"executableName,omitempty"`
	ConnectionStatus string `json:"connectionStatus"`
	NextStep         string `json:"nextStep"`
}

func detectTunnelClient() tunnelDiagnostic {
	return detectTunnelClientWith(exec.LookPath, os.Stat)
}

func detectTunnelClientWith(lookup func(string) (string, error), stat func(string) (os.FileInfo, error)) tunnelDiagnostic {
	result := tunnelDiagnostic{
		Checked:          true,
		ConnectionStatus: "not_checked",
		NextStep:         "No tunnel health or ChatGPT connectivity check has been performed.",
	}
	found, err := lookup("tunnel-client")
	if err != nil {
		result.NextStep = "Tunnel client was not found on PATH. It may be installed elsewhere. Follow the official tunnel setup guide."
		return result
	}
	info, err := stat(found)
	if err != nil || !info.Mode().IsRegular() {
		result.NextStep = "A tunnel-client PATH entry exists but is not a regular executable. Check installation manually."
		return result
	}
	result.ClientOnPath = true
	// Display only basename, not private home folders or install paths.
	result.ExecutableName = filepath.Base(found)
	result.NextStep = "Tunnel client is discoverable on PATH. Health, authorization and ChatGPT access remain unverified."
	return result
}

func (w wizard) handleTunnelDiagnostic(out http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(out, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Dirhaven-Setup-Token")), []byte(w.token)) != 1 {
		http.Error(out, "setup authorization required", http.StatusUnauthorized)
		return
	}
	check := detectTunnelClient()
	// An unexpected process environment must never be reflected verbatim.
	check.ExecutableName = strings.TrimSpace(check.ExecutableName)
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(out).Encode(check)
}
