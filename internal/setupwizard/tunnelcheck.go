package setupwizard

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// inspectChosenTunnelClient checks metadata for a path explicitly supplied by
// the user. The filename is only a naming hint, not an authenticity check.
// It never executes the file, reads its contents, or returns its full path.
func inspectChosenTunnelClient(path string) tunnelDiagnostic {
	result := tunnelDiagnostic{
		Checked:          true,
		ConnectionStatus: "not_checked",
		NextStep:         "No tunnel process, profile, credentials or ChatGPT connection has been checked.",
	}
	if path == "" || len(path) > 4096 || strings.TrimSpace(path) != path || !filepath.IsAbs(path) {
		result.NextStep = "Choose an absolute file path to a tunnel-client executable."
		return result
	}
	name := filepath.Base(path)
	if (runtime.GOOS == "windows" && !strings.EqualFold(name, "tunnel-client.exe")) ||
		(runtime.GOOS != "windows" && name != "tunnel-client") {
		result.NextStep = "The selected filename is not tunnel-client. No authenticity verification was attempted."
		return result
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		result.NextStep = "The chosen entry is missing, a link, or not a regular file. No connection check was performed."
		return result
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		result.NextStep = "The chosen file is not marked executable. No connection check was performed."
		return result
	}
	result.ClientOnPath = false
	result.ExecutableName = name
	result.NextStep = "Chosen file found. Filename and filesystem metadata only; publisher, contents and connection remain unverified."
	return result
}

type selectedTunnelRequest struct {
	Path string `json:"path"`
}

func (w wizard) handleChosenTunnelDiagnostic(out http.ResponseWriter, req *http.Request) {
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
	req.Body = http.MaxBytesReader(out, req.Body, 8192)
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	var input selectedTunnelRequest
	if err := decoder.Decode(&input); err != nil {
		http.Error(out, "invalid request", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(out, "unexpected trailing content", http.StatusBadRequest)
		return
	}
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(out).Encode(inspectChosenTunnelClient(input.Path))
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
