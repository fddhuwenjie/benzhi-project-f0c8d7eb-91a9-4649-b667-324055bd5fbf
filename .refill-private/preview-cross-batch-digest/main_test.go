package preview_cross_batch_digest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
)

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func baseline(now time.Time) domain.Baseline {
	return domain.Baseline{TelescopeID: "RT", TargetSource: "M87", FrequencyBand: "L", FrequencyLowHz: 1e9, FrequencyHighHz: 2e9, PlannedWindow: domain.PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 600, QualityThresholds: domain.QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .9, MaxPacketLoss: .1, MinSignalToNoise: 5}, SamplingSeed: "seed", SampleSize: 1}
}

func seg(id, contentHash string, now time.Time, rfi float64) domain.ObservationSegment {
	return domain.ObservationSegment{SegmentID: id, StartTime: now, EndTime: now.Add(10 * time.Minute), FrequencyLowHz: 1.1e9, FrequencyHighHz: 1.9e9, ValidDurationSeconds: 600, FileReference: "local://" + id, ContentSHA256: contentHash, SubmittedBy: "operator", Metrics: domain.SegmentMetrics{RFIOccupancy: rfi, Completeness: .99, PacketLoss: .01, SignalToNoise: 12}}
}

func createFrozen(t *testing.T, app *application.Service, id string, now time.Time) *domain.ObservationBatch {
	t.Helper()
	created, err := app.CreateBatch(application.CreateBatchCommand{RequestMeta: application.RequestMeta{RequestID: "create-" + id, Actor: "operator"}, BatchID: id})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := app.FreezeBaseline(id, application.FreezeBaselineCommand{RequestMeta: application.RequestMeta{RequestID: "freeze-" + id, ExpectedRevision: created.Batch.Revision, Actor: "operator"}, Baseline: baseline(now)})
	if err != nil {
		t.Fatal(err)
	}
	return frozen.Batch
}

func TestPreviewCrossBatchDigest(t *testing.T) {
	now := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	repo, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(repo)
	shared := hash("same-observation-content")
	owner := createFrozen(t, app, "B-OWNER", now)
	if _, err := app.RegisterSegment(owner.BatchID, application.RegisterSegmentCommand{RequestMeta: application.RequestMeta{RequestID: "owner-segment", ExpectedRevision: owner.Revision, Actor: "operator"}, Segment: seg("SEG-OWNER", shared, now, .1)}); err != nil {
		t.Fatal(err)
	}
	target := createFrozen(t, app, "B-TARGET", now)
	registered, err := app.RegisterSegment(target.BatchID, application.RegisterSegmentCommand{RequestMeta: application.RequestMeta{RequestID: "bad-segment", ExpectedRevision: target.Revision, Actor: "operator"}, Segment: seg("SEG-BAD", hash("bad"), now, .8)})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := app.AssessSegment(target.BatchID, application.AssessSegmentCommand{RequestMeta: application.RequestMeta{RequestID: "assess-bad", ExpectedRevision: registered.Batch.Revision, Actor: "operator"}, SegmentID: "SEG-BAD"})
	if err != nil {
		t.Fatal(err)
	}
	quarantined, err := app.Quarantine(target.BatchID, application.QuarantineCommand{RequestMeta: application.RequestMeta{RequestID: "quarantine-bad", ExpectedRevision: assessed.Batch.Revision, Actor: "operator"}, SegmentID: "SEG-BAD", Reason: "干扰", ReobservationPlan: "补观"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.PreviewReplacement(target.BatchID, application.PreviewReplacementCommand{ExpectedRevision: quarantined.Batch.Revision, IssueID: "issue-SEG-BAD", Segment: seg("SEG-PREVIEW", shared, now, .1)})
	var appErr *application.Error
	if !errors.As(err, &appErr) || appErr.Code != "cross_batch_digest" {
		t.Fatalf("TestPreviewCrossBatchDigest: preview accepted content owned by another batch: %v", err)
	}
}
