package setupwizard

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxTunnelFingerprintSize int64 = 256 << 20

type tunnelFingerprintRequest struct {
	Path string `json:"path"`
}
type tunnelFingerprintResponse struct {
	OK           bool   `json:"ok"`
	SHA256       string `json:"sha256,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	Authenticity string `json:"authenticity"`
	Error        string `json:"error,omitempty"`
}

// fingerprintTunnelFile reads only the explicitly selected candidate executable
// for a SHA-256 fingerprint. It neither runs the file nor validates its publisher.
// No reference checksum is trusted or supplied by Dirhaven.
func fingerprintTunnelFile(path string) (string, int64, error) {
	d := inspectChosenTunnelClient(path)
	if d.ExecutableName == "" {
		return "", 0, errors.New("chosen file is not an eligible tunnel-client executable")
	}
	if strings.TrimSpace(path) != path || !filepath.IsAbs(path) {
		return "", 0, errors.New("invalid file path")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, errors.New("cannot open selected file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTunnelFingerprintSize {
		return "", 0, errors.New("selected file is not regular or exceeds the 256 MiB inspection limit")
	}
	if info.Size() < 0 {
		return "", 0, errors.New("invalid selected file size")
	}
	sum := sha256.New()
	length, err := io.Copy(sum, io.LimitReader(f, maxTunnelFingerprintSize+1))
	if err != nil || length > maxTunnelFingerprintSize {
		return "", 0, errors.New("could not fingerprint selected file")
	}
	// Reject a truncated or changed file rather than displaying misleading data.
	after, err := f.Stat()
	if err != nil || length != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return "", 0, errors.New("selected file changed during inspection")
	}
	return hex.EncodeToString(sum.Sum(nil)), length, nil
}

func (w wizard) handleTunnelFingerprint(out http.ResponseWriter, req *http.Request) {
	if !w.authorizedTunnelRequest(out, req) {
		return
	}
	var input tunnelFingerprintRequest
	if !decodeSavedJSON(out, req, &input) {
		return
	}
	if len(input.Path) > 4096 {
		http.Error(out, "invalid selected path", http.StatusBadRequest)
		return
	}
	digest, size, err := fingerprintTunnelFile(input.Path)
	result := tunnelFingerprintResponse{Authenticity: "not_verified"}
	if err != nil {
		result.Error = "Unable to fingerprint this file. No authenticity claim was made."
	} else {
		result.OK = true
		result.SHA256 = digest
		result.SizeBytes = size
	}
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(out).Encode(result)
}

// Only an authorized user-initiated POST can trigger reading the selected file.
func (w wizard) authorizedTunnelRequest(out http.ResponseWriter, req *http.Request) bool {
	if req.Method != http.MethodPost {
		http.Error(out, "POST required", http.StatusMethodNotAllowed)
		return false
	}
	if w.token == "" || subtle.ConstantTimeCompare([]byte(req.Header.Get("X-Dirhaven-Setup-Token")), []byte(w.token)) != 1 {
		http.Error(out, "setup authorization required", http.StatusUnauthorized)
		return false
	}
	return true
}
