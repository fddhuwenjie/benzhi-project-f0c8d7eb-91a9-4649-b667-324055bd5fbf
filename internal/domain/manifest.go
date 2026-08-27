package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func CanonicalJSON(v any) ([]byte, error) { return json.Marshal(v) }

func Digest(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

type manifestBody struct {
	ManifestID       string            `json:"manifest_id"`
	BatchID          string            `json:"batch_id"`
	TerminalDecision string            `json:"terminal_decision"`
	SegmentDigests   []ManifestSegment `json:"segment_digests"`
	BaselineDigest   string            `json:"baseline_digest"`
	ReviewDigest     string            `json:"review_digest"`
	SealedBy         string            `json:"sealed_by"`
	SealedAt         time.Time         `json:"sealed_at"`
}

func (b *ObservationBatch) Seal(actor string, now time.Time) (*ReleaseManifest, error) {
	if b.Terminal() {
		return nil, ErrTerminal
	}
	if b.State != StateReview || !b.ReviewComplete() {
		return nil, rule("review_incomplete", "全部抽审项目裁定后才能封存")
	}
	if strings.TrimSpace(actor) == "" {
		return nil, rule("sealer_required", "封存人不能为空")
	}
	decision := "approved"
	if !b.ReviewPassed() {
		decision = "rejected"
	}
	segments := make([]ManifestSegment, 0)
	for _, s := range b.Segments {
		if s.Status == SegmentPassed {
			segments = append(segments, ManifestSegment{s.SegmentID, s.FileReference, strings.ToLower(s.ContentSHA256)})
		}
	}
	sort.Slice(segments, func(i, j int) bool { return segments[i].SegmentID < segments[j].SegmentID })
	baselineDigest, err := Digest(b.Baseline)
	if err != nil {
		return nil, err
	}
	reviewDigest, err := Digest(b.ReviewItems)
	if err != nil {
		return nil, err
	}
	body := manifestBody{ManifestID: "manifest-" + b.BatchID, BatchID: b.BatchID, TerminalDecision: decision, SegmentDigests: segments, BaselineDigest: baselineDigest, ReviewDigest: reviewDigest, SealedBy: actor, SealedAt: now.UTC()}
	canonical, err := Digest(body)
	if err != nil {
		return nil, err
	}
	m := &ReleaseManifest{ManifestID: body.ManifestID, BatchID: b.BatchID, TerminalDecision: decision, SegmentDigests: segments, BaselineDigest: baselineDigest, ReviewDigest: reviewDigest, CanonicalSHA256: canonical, SealedBy: actor, SealedAt: body.SealedAt}
	b.Manifest = m
	if decision == "approved" {
		b.State = StateApproved
	} else {
		b.State = StateRejected
	}
	b.touch(now)
	return m, nil
}

func VerifyManifest(m ReleaseManifest) (string, bool, error) {
	body := manifestBody{m.ManifestID, m.BatchID, m.TerminalDecision, m.SegmentDigests, m.BaselineDigest, m.ReviewDigest, m.SealedBy, m.SealedAt}
	digest, err := Digest(body)
	return digest, err == nil && strings.EqualFold(digest, m.CanonicalSHA256), err
}
