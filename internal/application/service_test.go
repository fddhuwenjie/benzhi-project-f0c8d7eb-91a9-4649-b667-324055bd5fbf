package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	repo, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(repo)
	fixed := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	service.clock = func() time.Time { return fixed }
	return service
}

func createAndFreeze(t *testing.T, service *Service, id string) (*domain.ObservationBatch, domain.Baseline) {
	t.Helper()
	created, err := service.CreateBatch(CreateBatchCommand{RequestMeta: RequestMeta{RequestID: "create-" + id, ExpectedRevision: 0, Actor: "operator"}, BatchID: id})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	baseline := domain.Baseline{TelescopeID: "RT-01", TargetSource: "M87", FrequencyBand: "L", FrequencyLowHz: 1e9, FrequencyHighHz: 2e9, PlannedWindow: domain.PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 300, QualityThresholds: domain.QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .9, MaxPacketLoss: .1, MinSignalToNoise: 5}, SamplingSeed: "seed", SampleSize: 1}
	frozen, err := service.FreezeBaseline(id, FreezeBaselineCommand{RequestMeta: RequestMeta{RequestID: "freeze-" + id, ExpectedRevision: created.Batch.Revision, Actor: "operator"}, Baseline: baseline})
	if err != nil {
		t.Fatal(err)
	}
	return frozen.Batch, baseline
}

func appSegment(id string, baseline domain.Baseline, digest string) domain.ObservationSegment {
	return domain.ObservationSegment{SegmentID: id, StartTime: baseline.PlannedWindow.Start, EndTime: baseline.PlannedWindow.Start.Add(10 * time.Minute), FrequencyLowHz: 1.1e9, FrequencyHighHz: 1.9e9, ValidDurationSeconds: 500, FileReference: "local://" + id, ContentSHA256: digest, SubmittedBy: "operator", Metrics: domain.SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 10}}
}

func TestIdempotentReplayAndStaleRevision(t *testing.T) {
	service := newTestService(t)
	command := CreateBatchCommand{RequestMeta: RequestMeta{RequestID: "create-one", ExpectedRevision: 0, Actor: "operator"}, BatchID: "B-ONE"}
	first, err := service.CreateBatch(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateBatch(command)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.Batch.Revision != first.Batch.Revision {
		t.Fatalf("request was not replayed: %#v", second)
	}
	changed := command
	changed.Actor = "other"
	if _, err := service.CreateBatch(changed); err == nil {
		t.Fatal("idempotency mismatch accepted")
	}
	now := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	baseline := domain.Baseline{TelescopeID: "RT", TargetSource: "M87", FrequencyBand: "L", FrequencyLowHz: 1, FrequencyHighHz: 2, PlannedWindow: domain.PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 10, QualityThresholds: domain.QualityThresholds{}, SamplingSeed: "seed", SampleSize: 1}
	_, err = service.FreezeBaseline("B-ONE", FreezeBaselineCommand{RequestMeta: RequestMeta{RequestID: "stale", ExpectedRevision: 0, Actor: "operator"}, Baseline: baseline})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != "stale_revision" || appErr.CurrentRevision != 1 {
		t.Fatalf("unexpected stale error: %#v", err)
	}
}

func TestCrossBatchContentDigestRejected(t *testing.T) {
	service := newTestService(t)
	first, baseline := createAndFreeze(t, service, "B-FIRST")
	second, _ := createAndFreeze(t, service, "B-SECOND")
	sum := sha256.Sum256([]byte("same-content"))
	digest := hex.EncodeToString(sum[:])
	_, err := service.RegisterSegment(first.BatchID, RegisterSegmentCommand{RequestMeta: RequestMeta{RequestID: "segment-first", ExpectedRevision: first.Revision, Actor: "operator"}, Segment: appSegment("SEG-1", baseline, digest)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RegisterSegment(second.BatchID, RegisterSegmentCommand{RequestMeta: RequestMeta{RequestID: "segment-second", ExpectedRevision: second.Revision, Actor: "operator"}, Segment: appSegment("SEG-2", baseline, digest)})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != "cross_batch_digest" {
		t.Fatalf("unexpected duplicate error: %#v", err)
	}
}

func TestBulkRegistrationAndBatchQualityAreAtomicAndIdempotent(t *testing.T) {
	service := newTestService(t)
	batch, baseline := createAndFreeze(t, service, "B-BULK")
	segments := make([]domain.ObservationSegment, 3)
	for i := range segments {
		sum := sha256.Sum256([]byte{byte(i + 1)})
		segments[i] = appSegment(fmt.Sprintf("SEG-%d", i+1), baseline, hex.EncodeToString(sum[:]))
		segments[i].StartTime = baseline.PlannedWindow.Start.Add(time.Duration(i) * 10 * time.Minute)
		segments[i].EndTime = segments[i].StartTime.Add(10 * time.Minute)
	}
	cmd := RegisterSegmentsCommand{RequestMeta: RequestMeta{RequestID: "bulk-register", ExpectedRevision: batch.Revision, Actor: "operator"}, Segments: segments}
	registered, err := service.RegisterSegments(batch.BatchID, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if registered.Batch.Revision != batch.Revision+1 || len(registered.Registered) != 3 {
		t.Fatalf("unexpected bulk result: %#v", registered)
	}
	replayed, err := service.RegisterSegments(batch.BatchID, cmd)
	if err != nil || !replayed.Replayed || replayed.Batch.Revision != registered.Batch.Revision {
		t.Fatalf("bulk replay failed: %#v %v", replayed, err)
	}
	qualityCmd := AssessBatchCommand{RequestMeta: RequestMeta{RequestID: "bulk-quality", ExpectedRevision: registered.Batch.Revision, Actor: "operator"}}
	quality, err := service.AssessBatch(batch.BatchID, qualityCmd)
	if err != nil {
		t.Fatal(err)
	}
	if quality.Batch.Revision != registered.Batch.Revision+1 || quality.QualitySummary.Total != 3 || quality.QualitySummary.Passed != 3 {
		t.Fatalf("unexpected quality result: %#v", quality)
	}
	qualityReplay, err := service.AssessBatch(batch.BatchID, qualityCmd)
	if err != nil || !qualityReplay.Replayed || qualityReplay.Batch.Revision != quality.Batch.Revision {
		t.Fatalf("quality replay failed: %#v %v", qualityReplay, err)
	}
	events, err := service.repo.Events(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("expected four audit events, got %d", len(events))
	}
}

func TestBulkValidationReturnsRowsWithoutMutation(t *testing.T) {
	service := newTestService(t)
	batch, baseline := createAndFreeze(t, service, "B-ATOMIC")
	other, _ := createAndFreeze(t, service, "B-OWNER")
	crossSum := sha256.Sum256([]byte("owned"))
	crossDigest := hex.EncodeToString(crossSum[:])
	if _, err := service.RegisterSegment(other.BatchID, RegisterSegmentCommand{RequestMeta: RequestMeta{RequestID: "owner-segment", ExpectedRevision: other.Revision, Actor: "operator"}, Segment: appSegment("OWNER-SEG", baseline, crossDigest)}); err != nil {
		t.Fatal(err)
	}
	dupSum := sha256.Sum256([]byte("duplicate"))
	dup := hex.EncodeToString(dupSum[:])
	segments := []domain.ObservationSegment{appSegment("SEG-A", baseline, dup), appSegment("SEG-B", baseline, dup), appSegment("SEG-C", baseline, crossDigest)}
	_, err := service.RegisterSegments(batch.BatchID, RegisterSegmentsCommand{RequestMeta: RequestMeta{RequestID: "invalid-bulk", ExpectedRevision: batch.Revision, Actor: "operator"}, Segments: segments})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != "bulk_validation_failed" {
		t.Fatalf("unexpected error: %#v", err)
	}
	details, ok := appErr.Details.([]BulkItemError)
	if !ok || len(details) != 2 || details[0].InputIndex != 1 || details[1].InputIndex != 2 {
		t.Fatalf("unexpected details: %#v", appErr.Details)
	}
	view, err := service.GetBatch(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Batch.Revision != batch.Revision || len(view.Batch.Segments) != 0 {
		t.Fatalf("bulk failure mutated batch: %#v", view.Batch)
	}
	events, _ := service.repo.Events(batch.BatchID)
	if len(events) != 2 {
		t.Fatalf("bulk failure wrote event: %d", len(events))
	}
}
