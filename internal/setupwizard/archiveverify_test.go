package setupwizard

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveAgainstLocalManifest(t *testing.T) {
	dir := t.TempDir()
	name := "tunnel-client-v0.0.20-windows-amd64.zip"
	archive := filepath.Join(dir, name)
	sums := filepath.Join(dir, "SHA256SUMS.txt")
	bytes := []byte("example ZIP payload - test only")
	if err := os.WriteFile(archive, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bytes)
	expected := hex.EncodeToString(digest[:])
	manifest := expected + "  " + name + "\n"
	if err := os.WriteFile(sums, []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	verified, err := verifyArchiveAgainstLocalManifest(archive, sums)
	if err != nil || !verified.Matches || verified.Authenticity != "not_verified" || verified.ArchiveSHA256 != expected || verified.ManifestSHA256 != expected {
		t.Fatalf("unexpected checksum result: %+v / %v", verified, err)
	}
	if err := os.WriteFile(archive, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	mismatched, err := verifyArchiveAgainstLocalManifest(archive, sums)
	if err != nil || mismatched.Matches || mismatched.Authenticity != "not_verified" {
		t.Fatalf("hash mismatch was mistaken for success: %+v / %v", mismatched, err)
	}
	for _, contents := range []string{
		expected + "  " + name + "\n" + expected + "  " + name + "\n",
		expected + "  another.zip\n",
		strings.Repeat("z", 64) + "  " + name + "\n",
	} {
		if err := os.WriteFile(sums, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyArchiveAgainstLocalManifest(archive, sums); err == nil {
			t.Errorf("accepted invalid manifest %q", contents)
		}
	}
}

func TestArchiveVerificationRefusesSymlinksAndOtherFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "tunnel-client-v0.0.20-linux-arm64.zip")
	manifest := filepath.Join(dir, "SHA256SUMS.txt")
	_ = os.WriteFile(archive, []byte("archive"), 0600)
	_ = os.WriteFile(manifest, []byte(strings.Repeat("a", 64)+"  "+filepath.Base(archive)+"\n"), 0600)
	link := filepath.Join(dir, "other.zip")
	if err := os.Symlink(archive, link); err == nil {
		if _, err := verifyArchiveAgainstLocalManifest(link, manifest); err == nil {
			t.Fatal("symlink with unsupported name accepted")
		}
		if err := os.Remove(archive); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(link, archive); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyArchiveAgainstLocalManifest(archive, manifest); err == nil {
			t.Fatal("symlink archive accepted")
		}
	}
}

func TestArchiveEndpointAuthorizationAndNoProvenanceClaims(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("h", 64)}
	for _, tc := range []struct {
		method, token, origin string
		status                int
	}{
		{http.MethodGet, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "", "", http.StatusUnauthorized},
		{http.MethodPost, "incorrect", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "https://evil.example", http.StatusForbidden},
		{http.MethodPost, w.token, "http://" + w.address, http.StatusOK},
	} {
		response := requestWizard(t, w, tc.method, "/api/tunnel-archive-verify", tc.token, tc.origin, archiveVerifyRequest{ArchivePath: "relative", ManifestPath: "relative"})
		if response.Code != tc.status {
			t.Fatalf("got %d vs expected %d", response.Code, tc.status)
		}
		if tc.status == http.StatusOK && (!strings.Contains(response.Body.String(), `"authenticity":"not_verified"`) || strings.Contains(response.Body.String(), `"matches":true`)) {
			t.Fatalf("invalid archive got provenance claim: %s", response.Body.String())
		}
	}
}
