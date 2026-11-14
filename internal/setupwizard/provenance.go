package setupwizard

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Sigstore verification is opt-in. This code never executes tunnel-client,
// uses local tunnel profiles, opens ports or modifies any credentials.
// The GitHub CLI may obtain Sigstore trusted-root data from the network.
const maxProvenanceBundleSize int64 = 32 << 20

var tunnelReleaseArchivePattern = regexp.MustCompile(`^tunnel-client(?:-runtime(?:-cloudflared)?)?-(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)-(?:windows|darwin|linux)-(?:amd64|arm64)\.zip$`)

type provenanceRequest struct {
	ArchivePath  string `json:"archivePath"`
	BundlePath   string `json:"bundlePath"`
	SourceDigest string `json:"sourceDigest"`
}

type provenanceResponse struct {
	Checked            bool   `json:"checked"`
	SignatureVerified  bool   `json:"signatureVerified"`
	SourceDigestPinned bool   `json:"sourceDigestPinned"`
	Identity           string `json:"identity"`
	ReleaseTag         string `json:"releaseTag,omitempty"`
	ConnectionStatus   string `json:"connectionStatus"`
	Error              string `json:"error,omitempty"`
}

type attestationRunner func(context.Context, []string) ([]byte, error)

func githubAttestationRunner(ctx context.Context, args []string) ([]byte, error) {
	// No shell expansion and no untrusted command strings.
	command := exec.CommandContext(ctx, "gh", args...)
	return command.Output()
}

func verifySignedRelease(parent context.Context, in provenanceRequest, runner attestationRunner) provenanceResponse {
	result := provenanceResponse{
		Checked:          true,
		Identity:         "openai/tunnel-client/.github/workflows/release.yml",
		ConnectionStatus: "not_checked",
	}
	if len(in.ArchivePath) == 0 || len(in.ArchivePath) > 4096 ||
		len(in.BundlePath) == 0 || len(in.BundlePath) > 4096 ||
		strings.TrimSpace(in.ArchivePath) != in.ArchivePath ||
		strings.TrimSpace(in.BundlePath) != in.BundlePath ||
		!filepath.IsAbs(in.ArchivePath) || !filepath.IsAbs(in.BundlePath) {
		result.Error = "Select absolute paths to an official release ZIP and its Sigstore provenance bundle."
		return result
	}
	archiveName := filepath.Base(in.ArchivePath)
	match := tunnelReleaseArchivePattern.FindStringSubmatch(archiveName)
	if len(match) != 2 || !officialArchiveName.MatchString(archiveName) {
		result.Error = "The selected archive does not match a supported tunnel-client release name."
		return result
	}
	release := match[1]
	result.ReleaseTag = release
	if filepath.Base(in.BundlePath) != "tunnel-client-"+release+"-provenance.sigstore.json" {
		result.Error = "The selected Sigstore bundle does not belong to the ZIP release tag."
		return result
	}
	// Optional pin: a 40-character Git commit SHA obtained independently from
	// the public release tag. Omitting it reduces the scope of the check.
	source := strings.ToLower(strings.TrimSpace(in.SourceDigest))
	if source != "" {
		if len(source) != 40 {
			result.Error = "Source commit must be a 40-character Git SHA-1 digest."
			return result
		}
		if _, err := hex.DecodeString(source); err != nil {
			result.Error = "Source commit must be a hexadecimal Git SHA-1 digest."
			return result
		}
		result.SourceDigestPinned = true
	}
	archiveInfo, err := os.Lstat(in.ArchivePath)
	if err != nil || !archiveInfo.Mode().IsRegular() || archiveInfo.Size() < 1 || archiveInfo.Size() > maxTunnelArchiveSize {
		result.Error = "The selected ZIP is unavailable or exceeds the allowed archive size."
		return result
	}
	bundleInfo, err := os.Lstat(in.BundlePath)
	if err != nil || !bundleInfo.Mode().IsRegular() || bundleInfo.Size() < 1 || bundleInfo.Size() > maxProvenanceBundleSize {
		result.Error = "The selected provenance bundle is unavailable or exceeds the allowed size."
		return result
	}
	if runner == nil {
		result.Error = "The signed provenance verifier is unavailable."
		return result
	}
	args := []string{
		"attestation", "verify", in.ArchivePath,
		"--bundle", in.BundlePath,
		"--repo", "openai/tunnel-client",
		"--signer-workflow", result.Identity,
		"--source-ref", "refs/tags/" + release,
		"--predicate-type", "https://slsa.dev/provenance/v1",
		"--deny-self-hosted-runners",
		"--format", "json",
	}
	if source != "" {
		args = append(args, "--source-digest", source, "--signer-digest", source)
	}
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	output, err := runner(ctx, args)
	if err != nil {
		result.Error = "Signed provenance could not be verified. Install a trusted GitHub CLI, check the bundle, and retry. No tunnel was started."
		return result
	}
	if len(output) > 4<<20 {
		result.Error = "The verifier returned an unexpectedly large response."
		return result
	}
	// Never trust a process exit code alone. The GitHub CLI must return at
	// least one verified statement for this ZIP and its SHA-256 digest.
	var data []struct {
		VerificationResult struct {
			Statement struct {
				Subject []struct {
					Name   string            `json:"name"`
					Digest map[string]string `json:"digest"`
				} `json:"subject"`
			} `json:"statement"`
		} `json:"verificationResult"`
	}
	if err := json.Unmarshal(output, &data); err != nil || len(data) == 0 {
		result.Error = "The verifier did not return a valid signed-attestation result."
		return result
	}
	// The external CLI verifies its signature/identity policy, and here we
	// independently confirm that its reported subject covers the actual ZIP.
	zipFile, err := os.Open(in.ArchivePath)
	if err != nil {
		result.Error = "The selected ZIP could not be re-opened for a final digest check."
		return result
	}
	defer zipFile.Close()
	info, err := zipFile.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != archiveInfo.Size() || !os.SameFile(info, archiveInfo) {
		result.Error = "The release ZIP changed during verification."
		return result
	}
	hash := sha256.New()
	copied, err := io.Copy(hash, io.LimitReader(zipFile, maxTunnelArchiveSize+1))
	if err != nil || copied != archiveInfo.Size() {
		result.Error = "The release ZIP changed during verification."
		return result
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	after, err := zipFile.Stat()
	if err != nil || after.Size() != archiveInfo.Size() || !after.ModTime().Equal(archiveInfo.ModTime()) {
		result.Error = "The release ZIP changed during verification."
		return result
	}
	for _, entry := range data {
		for _, subject := range entry.VerificationResult.Statement.Subject {
			if subject.Name == archiveName && len(subject.Digest["sha256"]) == 64 &&
				subtle.ConstantTimeCompare([]byte(strings.ToLower(subject.Digest["sha256"])), []byte(digest)) == 1 {
				result.SignatureVerified = true
				return result
			}
		}
	}
	result.Error = "No verified attestation subject matched the selected ZIP's current SHA-256."
	return result
}

func (w wizard) handleProvenance(out http.ResponseWriter, req *http.Request) {
	if !w.authorizedTunnelRequest(out, req) {
		return
	}
	var input provenanceRequest
	if !decodeSavedJSON(out, req, &input) {
		return
	}
	result := verifySignedRelease(req.Context(), input, githubAttestationRunner)
	out.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(out).Encode(result)
}
