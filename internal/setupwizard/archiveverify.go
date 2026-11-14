package setupwizard

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxTunnelArchiveSize int64 = 256 << 20
const maxTunnelManifestSize int64 = 1 << 20

var officialArchiveName = regexp.MustCompile(`^tunnel-client(?:-runtime(?:-cloudflared)?)?-v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?-(?:windows|darwin|linux)-(?:amd64|arm64)\.zip$`)

type archiveVerifyRequest struct {
	ArchivePath  string `json:"archivePath"`
	ManifestPath string `json:"manifestPath"`
}
type archiveVerifyResponse struct {
	Checked        bool   `json:"checked"`
	Matches        bool   `json:"matches"`
	ArchiveName    string `json:"archiveName,omitempty"`
	ArchiveSHA256  string `json:"archiveSHA256,omitempty"`
	ManifestSHA256 string `json:"manifestSHA256,omitempty"`
	Authenticity   string `json:"authenticity"`
	Error          string `json:"error,omitempty"`
}

// verifyArchiveAgainstLocalManifest checks only two user-selected files.
// It does NOT prove the manifest is from OpenAI or verify Sigstore provenance.
func verifyArchiveAgainstLocalManifest(archivePath, manifestPath string) (archiveVerifyResponse, error) {
	result := archiveVerifyResponse{Checked: true, Authenticity: "not_verified"}
	if len(archivePath) > 4096 || len(manifestPath) > 4096 ||
		!filepath.IsAbs(archivePath) || !filepath.IsAbs(manifestPath) ||
		strings.TrimSpace(archivePath) != archivePath || strings.TrimSpace(manifestPath) != manifestPath {
		return result, errors.New("select absolute archive and checksum file paths")
	}
	archiveName := filepath.Base(archivePath)
	if !officialArchiveName.MatchString(archiveName) || filepath.Base(manifestPath) != "SHA256SUMS.txt" {
		return result, errors.New("select a supported tunnel-client release ZIP and SHA256SUMS.txt")
	}
	result.ArchiveName = archiveName
	archive, err := os.Open(archivePath)
	if err != nil {
		return result, errors.New("cannot open selected archive")
	}
	defer archive.Close()
	archiveInfo, err := os.Lstat(archivePath)
	if err != nil || !archiveInfo.Mode().IsRegular() || archiveInfo.Size() < 1 || archiveInfo.Size() > maxTunnelArchiveSize {
		return result, errors.New("archive is not a regular file within the allowed size")
	}
	openedInfo, err := archive.Stat()
	if err != nil || !os.SameFile(archiveInfo, openedInfo) {
		return result, errors.New("archive changed before inspection")
	}
	manifest, err := os.Open(manifestPath)
	if err != nil {
		return result, errors.New("cannot open selected checksum manifest")
	}
	defer manifest.Close()
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Size() < 1 || manifestInfo.Size() > maxTunnelManifestSize {
		return result, errors.New("checksum manifest is not a regular file within the allowed size")
	}
	openedManifestInfo, err := manifest.Stat()
	if err != nil || !os.SameFile(manifestInfo, openedManifestInfo) {
		return result, errors.New("checksum manifest changed before inspection")
	}
	manifestBytes, err := io.ReadAll(io.LimitReader(manifest, maxTunnelManifestSize+1))
	if err != nil || int64(len(manifestBytes)) != manifestInfo.Size() {
		return result, errors.New("checksum manifest changed during read")
	}
	expected, err := parseArchiveChecksum(string(manifestBytes), archiveName)
	if err != nil {
		return result, err
	}
	hash := sha256.New()
	read, err := io.Copy(hash, io.LimitReader(archive, maxTunnelArchiveSize+1))
	if err != nil || read != archiveInfo.Size() {
		return result, errors.New("archive changed during hashing")
	}
	after, err := archive.Stat()
	if err != nil || after.Size() != archiveInfo.Size() || !after.ModTime().Equal(archiveInfo.ModTime()) {
		return result, errors.New("archive changed during inspection")
	}
	result.ArchiveSHA256 = hex.EncodeToString(hash.Sum(nil))
	result.ManifestSHA256 = expected
	result.Matches = subtle.ConstantTimeCompare([]byte(result.ArchiveSHA256), []byte(expected)) == 1
	return result, nil
}

func parseArchiveChecksum(manifest, archiveName string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(manifest))
	scanner.Buffer(make([]byte, 0, 8192), 65536)
	found := ""
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 {
			continue
		}
		hash := line[:64]
		if line[64] != ' ' || (line[65] != ' ' && line[65] != '*') {
			continue
		}
		name := line[66:]
		if name != archiveName {
			continue
		}
		if found != "" {
			return "", errors.New("checksum manifest has duplicate entries for the selected archive")
		}
		if len(hash) != 64 {
			return "", errors.New("checksum has invalid length")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return "", errors.New("checksum manifest contains invalid digest")
		}
		found = strings.ToLower(hash)
	}
	if err := scanner.Err(); err != nil {
		return "", errors.New("checksum manifest contains an unreadable line")
	}
	if found == "" {
		return "", errors.New("selected archive is not listed exactly once in the checksum manifest")
	}
	return found, nil
}

func (w wizard) handleArchiveVerify(out http.ResponseWriter, req *http.Request) {
	if !w.authorizedTunnelRequest(out, req) {
		return
	}
	var input archiveVerifyRequest
	if !decodeSavedJSON(out, req, &input) {
		return
	}
	result, err := verifyArchiveAgainstLocalManifest(input.ArchivePath, input.ManifestPath)
	if err != nil {
		result.Error = err.Error()
	}
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(out).Encode(result)
}
