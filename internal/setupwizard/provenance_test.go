package setupwizard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func provenanceFixtures(t *testing.T) (provenanceRequest, string) {
	t.Helper()
	dir := t.TempDir()
	name := "tunnel-client-runtime-v0.0.20-windows-amd64.zip"
	archive := filepath.Join(dir, name)
	bundle := filepath.Join(dir, "tunnel-client-v0.0.20-provenance.sigstore.json")
	data := []byte("test archive bytes; this fixture is intentionally not a real ZIP")
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return provenanceRequest{ArchivePath: archive, BundlePath: bundle}, hex.EncodeToString(sum[:])
}

func fakeVerifiedJSON(name, hash string) []byte {
	payload, _ := json.Marshal([]map[string]any{
		{"verificationResult": map[string]any{
			"statement": map[string]any{
				"subject": []map[string]any{{"name": name, "digest": map[string]any{"sha256": hash}}},
			},
		}},
	})
	return payload
}

func TestOptInProvenancePolicyAndSubjectMatching(t *testing.T) {
	input, digest := provenanceFixtures(t)
	calls := 0
	runner := func(_ context.Context, args []string) ([]byte, error) {
		calls++
		joined := strings.Join(args, " ")
		for _, expected := range []string{
			"attestation verify", "--bundle", "--repo openai/tunnel-client",
			"--signer-workflow openai/tunnel-client/.github/workflows/release.yml",
			"--source-ref refs/tags/v0.0.20",
			"--predicate-type https://slsa.dev/provenance/v1",
			"--deny-self-hosted-runners", "--format json",
		} {
			if !strings.Contains(joined, expected) {
				t.Errorf("verification omitted %q from args %q", expected, joined)
			}
		}
		if strings.Contains(joined, "doctor") || strings.Contains(joined, "tunnel-client run") {
			t.Fatal("verifier might start tunnel")
		}
		return fakeVerifiedJSON(filepath.Base(input.ArchivePath), digest), nil
	}
	good := verifySignedRelease(context.Background(), input, runner)
	if !good.SignatureVerified || good.SourceDigestPinned || good.ConnectionStatus != "not_checked" || calls != 1 {
		t.Fatalf("policy result inaccurate: %+v, calls=%d", good, calls)
	}
	input.SourceDigest = strings.Repeat("b", 40)
	withPin := verifySignedRelease(context.Background(), input, func(ctx context.Context, args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--source-digest "+input.SourceDigest) ||
			!strings.Contains(joined, "--signer-digest "+input.SourceDigest) {
			t.Errorf("provided release commit digest not enforced: %q", joined)
		}
		return runner(ctx, args)
	})
	if !withPin.SignatureVerified || !withPin.SourceDigestPinned {
		t.Fatalf("source commit pin lost: %+v", withPin)
	}
}

func TestFailClosedWhenAttestationUnverified(t *testing.T) {
	input, digest := provenanceFixtures(t)
	negative := []struct {
		name string
		run  attestationRunner
	}{
		{"external verifier failed", func(context.Context, []string) ([]byte, error) { return nil, errors.New("signature failed") }},
		{"invalid JSON", func(context.Context, []string) ([]byte, error) { return []byte("not json"), nil }},
		{"empty result", func(context.Context, []string) ([]byte, error) { return []byte("[]"), nil }},
		{"wrong subject", func(context.Context, []string) ([]byte, error) { return fakeVerifiedJSON("other.zip", digest), nil }},
		{"wrong hash", func(context.Context, []string) ([]byte, error) {
			return fakeVerifiedJSON(filepath.Base(input.ArchivePath), strings.Repeat("0", 64)), nil
		}},
	}
	for _, tt := range negative {
		t.Run(tt.name, func(t *testing.T) {
			got := verifySignedRelease(context.Background(), input, tt.run)
			if got.SignatureVerified || got.Error == "" {
				t.Fatalf("false provenance claim: %+v", got)
			}
		})
	}
}

func TestInvalidProvenanceSelectionNeverInvokesCLI(t *testing.T) {
	input, _ := provenanceFixtures(t)
	inputs := []provenanceRequest{
		{},
		{ArchivePath: "relative", BundlePath: input.BundlePath},
		{ArchivePath: input.ArchivePath, BundlePath: "relative"},
		{ArchivePath: input.ArchivePath, BundlePath: filepath.Join(filepath.Dir(input.ArchivePath), "wrong.sigstore.json")},
		{ArchivePath: input.ArchivePath, BundlePath: input.BundlePath, SourceDigest: "not-a-git-sha"},
	}
	for _, bad := range inputs {
		res := verifySignedRelease(context.Background(), bad, func(context.Context, []string) ([]byte, error) {
			t.Fatal("external CLI unexpectedly invoked")
			return nil, nil
		})
		if res.SignatureVerified || res.Error == "" {
			t.Fatalf("unsafe request accepted: %+v", bad)
		}
	}
}

// Optional end-to-end check: run only on an isolated CI runner that has
// downloaded a public reference release and a trusted GitHub CLI.
func TestPublicSignedProvenanceIntegration(t *testing.T) {
	if os.Getenv("DIRHAVEN_PROVENANCE_LIVE") != "1" {
		t.Skip("public signed-release integration is explicitly opt-in")
	}
	archive := os.Getenv("DIRHAVEN_PROVENANCE_ARCHIVE")
	bundle := os.Getenv("DIRHAVEN_PROVENANCE_BUNDLE")
	if archive == "" || bundle == "" {
		t.Fatal("public release files were not provided")
	}
	input := provenanceRequest{
		ArchivePath:  archive,
		BundlePath:   bundle,
		SourceDigest: "c2144b0904980bcbae2636cdca4f4167e9818e0b", // openai/tunnel-client v0.0.20 public tag
	}
	result := verifySignedRelease(context.Background(), input, githubAttestationRunner)
	if !result.SignatureVerified || !result.SourceDigestPinned || result.ReleaseTag != "v0.0.20" {
		t.Fatalf("reference release did not verify correctly: %+v", result)
	}
	// A wrong but well-formed source commit must fail closed.
	input.SourceDigest = strings.Repeat("f", 40)
	negative := verifySignedRelease(context.Background(), input, githubAttestationRunner)
	if negative.SignatureVerified || negative.Error == "" {
		t.Fatalf("invalid source digest was accepted: %+v", negative)
	}
}

func TestSignedProvenanceHTTPAuthorization(t *testing.T) {
	w := wizard{address: "127.0.0.1:35620", token: strings.Repeat("v", 64)}
	for _, tc := range []struct {
		method, token, origin string
		want                  int
	}{
		{http.MethodGet, w.token, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "", "", http.StatusUnauthorized},
		{http.MethodPost, "wrong", "", http.StatusUnauthorized},
		{http.MethodPost, w.token, "https://evil.example", http.StatusForbidden},
		{http.MethodPost, w.token, "http://" + w.address, http.StatusOK},
	} {
		got := requestWizard(t, w, tc.method, "/api/tunnel-provenance", tc.token, tc.origin, provenanceRequest{})
		if got.Code != tc.want {
			t.Fatalf("status %d; expected %d", got.Code, tc.want)
		}
		if got.Code == http.StatusOK && strings.Contains(got.Body.String(), `"signatureVerified":true`) {
			t.Fatalf("invalid request falsely marked verified: %s", got.Body.String())
		}
	}
}
