package setupwizard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var errPickerCancelled = errors.New("folder selection cancelled")

// chooseDirectory presents the OS directory picker on explicit user request.
// It never grants access or saves a path. Only the subsequent validated scope
// request may include the selected path in generated MCP configuration.
func chooseDirectory(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()

	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		command = "powershell.exe"
		// Constant script, no user-controlled data or interpolation.
		script := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = 'Choose a folder for Dirhaven'; $d.ShowNewFolderButton = $false; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.WriteLine($d.SelectedPath) } else { exit 2 }`
		args = []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", script}
	case "darwin":
		command = "osascript"
		args = []string{"-e", `POSIX path of (choose folder with prompt "Choose a folder for Dirhaven")`}
	default:
		switch {
		case hasExecutable("zenity"):
			command, args = "zenity", []string{"--file-selection", "--directory", "--title=Choose a folder for Dirhaven"}
		case hasExecutable("kdialog"):
			command, args = "kdialog", []string{"--getexistingdirectory", ".", "--title", "Choose a folder for Dirhaven"}
		default:
			return "", errors.New("no desktop folder picker available; paste an absolute path instead")
		}
	}

	cmd := exec.CommandContext(ctx, command, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("folder picker timed out or was interrupted")
		}
		// Dialog cancellation is a normal outcome; never treat it as
		// permission to grant a fallback directory.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if (runtime.GOOS == "windows" && exitErr.ExitCode() == 2) ||
				(runtime.GOOS == "darwin" && strings.Contains(stderr.String(), "-128")) ||
				(runtime.GOOS == "linux" && exitErr.ExitCode() == 1) {
				return "", errPickerCancelled
			}
		}
		return "", fmt.Errorf("folder picker unavailable: %w", err)
	}
	path := strings.TrimSpace(output.String())
	if path == "" {
		return "", errPickerCancelled
	}
	if len(path) > 8192 || !filepath.IsAbs(path) {
		return "", errors.New("folder picker returned an invalid directory")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("selected directory is not accessible")
	}
	return filepath.Clean(resolved), nil
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
