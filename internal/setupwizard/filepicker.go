package setupwizard

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// chooseTunnelExecutable opens an explicit OS file dialog. It never reads
// or executes the selected binary. Only the path is returned to the wizard.
// The separate metadata inspection remains opt-in after selection.
func chooseTunnelExecutable(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		command = "powershell.exe"
		script := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Title = 'Choose a tunnel-client executable'; $d.Filter = 'Applications (*.exe)|*.exe|All files (*.*)|*.*'; $d.CheckFileExists = $true; $d.Multiselect = $false; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.WriteLine($d.FileName) } else { exit 2 }`
		args = []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", script}
	case "darwin":
		command = "osascript"
		args = []string{"-e", `POSIX path of (choose file with prompt "Choose a tunnel-client executable")`}
	default:
		if hasExecutable("zenity") {
			command, args = "zenity", []string{"--file-selection", "--title=Choose a tunnel-client executable"}
		} else if hasExecutable("kdialog") {
			command, args = "kdialog", []string{"--getopenfilename", ".", "--title", "Choose a tunnel-client executable"}
		} else {
			return "", errors.New("no desktop file picker available; paste the absolute executable path")
		}
	}
	cmd := exec.CommandContext(ctx, command, args...)
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("file chooser interrupted or timed out")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) &&
			((runtime.GOOS == "windows" && exitErr.ExitCode() == 2) ||
				(runtime.GOOS == "darwin" && strings.Contains(stderr.String(), "-128")) ||
				(runtime.GOOS == "linux" && exitErr.ExitCode() == 1)) {
			return "", errPickerCancelled
		}
		return "", errors.New("file chooser unavailable; enter an absolute executable path manually")
	}
	path := strings.TrimSpace(output.String())
	if path == "" {
		return "", errPickerCancelled
	}
	if len(path) > 4096 || !filepath.IsAbs(path) {
		return "", errors.New("file chooser returned an invalid path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("selected file is missing or not a regular file")
	}
	return filepath.Clean(path), nil
}

func (w wizard) handlePickTunnelFile(out http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(out, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Dirhaven-Setup-Token")), []byte(w.token)) != 1 {
		http.Error(out, "setup authorization required", http.StatusUnauthorized)
		return
	}
	if w.pickTunnelFile == nil {
		http.Error(out, "file picker unavailable", http.StatusNotImplemented)
		return
	}
	if w.pickerBusy != nil && !w.pickerBusy.CompareAndSwap(false, true) {
		http.Error(out, "another chooser is already open", http.StatusConflict)
		return
	}
	if w.pickerBusy != nil {
		defer w.pickerBusy.Store(false)
	}
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	selected, err := w.pickTunnelFile(req.Context())
	if errors.Is(err, errPickerCancelled) {
		_ = json.NewEncoder(out).Encode(struct {
			Cancelled bool `json:"cancelled"`
		}{Cancelled: true})
		return
	}
	if err != nil {
		_ = json.NewEncoder(out).Encode(struct {
			Error string `json:"error"`
		}{Error: "Desktop file picker unavailable; paste the absolute path manually."})
		return
	}
	_ = json.NewEncoder(out).Encode(struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}{OK: true, Path: selected})
}
