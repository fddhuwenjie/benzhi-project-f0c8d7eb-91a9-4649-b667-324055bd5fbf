package manifest_verification_cache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
)

func TestManifestVerificationCacheInvalidation(t *testing.T) {
	repo, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest := signedManifest(t)
	if err := repo.SaveManifest(&manifest); err != nil {
		t.Fatal(err)
	}
	service := application.New(repo)
	first, err := service.VerifyManifest(manifest.BatchID)
	if err != nil || !first.Valid {
		t.Fatalf("initial manifest verification failed: %#v, %v", first, err)
	}

	manifest.SealedBy = "tampered-reviewer"
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo.Root(), "batches", manifest.BatchID, "manifest.json")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := service.VerifyManifest(manifest.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Valid {
		t.Fatalf("tampered manifest reused cached verification: %#v", second)
	}
}

func signedManifest(t *testing.T) domain.ReleaseManifest {
	t.Helper()
	sealedAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	segments := []domain.ManifestSegment{{
		SegmentID:     "SEG-001",
		FileReference: "local://SEG-001.fits",
		ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	body := struct {
		ManifestID       string                   `json:"manifest_id"`
		BatchID          string                   `json:"batch_id"`
		TerminalDecision string                   `json:"terminal_decision"`
		SegmentDigests   []domain.ManifestSegment `json:"segment_digests"`
		BaselineDigest   string                   `json:"baseline_digest"`
		ReviewDigest     string                   `json:"review_digest"`
		SealedBy         string                   `json:"sealed_by"`
		SealedAt         time.Time                `json:"sealed_at"`
	}{
		ManifestID:       "manifest-CACHE-001",
		BatchID:          "CACHE-001",
		TerminalDecision: "approved",
		SegmentDigests:   segments,
		BaselineDigest:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ReviewDigest:     "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		SealedBy:         "reviewer-a",
		SealedAt:         sealedAt,
	}
	digest, err := domain.Digest(body)
	if err != nil {
		t.Fatal(err)
	}
	return domain.ReleaseManifest{
		ManifestID:       body.ManifestID,
		BatchID:          body.BatchID,
		TerminalDecision: body.TerminalDecision,
		SegmentDigests:   body.SegmentDigests,
		BaselineDigest:   body.BaselineDigest,
		ReviewDigest:     body.ReviewDigest,
		CanonicalSHA256:  digest,
		SealedBy:         body.SealedBy,
		SealedAt:         body.SealedAt,
	}
}
