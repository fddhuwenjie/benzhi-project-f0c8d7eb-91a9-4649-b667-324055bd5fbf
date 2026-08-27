package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"
)

func testBaseline(now time.Time) Baseline {
	return Baseline{TelescopeID: "RT-01", TargetSource: "PSR-B0329+54", FrequencyBand: "L-band", FrequencyLowHz: 1.2e9, FrequencyHighHz: 1.6e9, PlannedWindow: PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 900, QualityThresholds: QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .95, MaxPacketLoss: .05, MinSignalToNoise: 8}, SamplingSeed: "fixed-seed", SampleSize: 1}
}

func testSegment(id string, now time.Time, metric SegmentMetrics) ObservationSegment {
	sum := sha256.Sum256([]byte(id))
	return ObservationSegment{SegmentID: id, StartTime: now, EndTime: now.Add(30 * time.Minute), FrequencyLowHz: 1.25e9, FrequencyHighHz: 1.55e9, ValidDurationSeconds: 1200, FileReference: "local://" + id + ".fits", ContentSHA256: hex.EncodeToString(sum[:]), SubmittedBy: "operator-a", Metrics: metric}
}

func TestRemediationReplacementAndSeal(t *testing.T) {
	now := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	batch, err := NewBatch("BATCH-1", "operator-a", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Freeze(testBaseline(now), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	failed := testSegment("SEG-BAD", now, SegmentMetrics{RFIOccupancy: .5, Completeness: .9, PacketLoss: .1, SignalToNoise: 4})
	if err := batch.AddSegment(failed, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	assessment, err := AssessSegment(batch, failed.SegmentID, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Decision != "failed" || len(assessment.IssueCodes) != 4 {
		t.Fatalf("unexpected assessment: %#v", assessment)
	}
	if issue := batch.Issues[failed.SegmentID]; issue == nil || !issue.Open {
		t.Fatal("failed quality did not open issue")
	}
	if err := batch.Quarantine(failed.SegmentID, "宽带干扰", "下一时窗补观", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	replacement := testSegment("SEG-GOOD", now, SegmentMetrics{RFIOccupancy: .05, Completeness: .99, PacketLoss: .01, SignalToNoise: 15})
	replacement.ReplacesSegmentID = failed.SegmentID
	if err := batch.AddSegment(replacement, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := AssessSegment(batch, replacement.SegmentID, now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if batch.Segments[failed.SegmentID].Status != SegmentReplaced || batch.Issues[failed.SegmentID].Open {
		t.Fatal("replacement did not close issue")
	}
	eligibility := EvaluateEligibility(batch)
	if !eligibility.CanGenerateReview || !eligibility.Coverage.Sufficient {
		t.Fatalf("batch not eligible: %#v", eligibility)
	}
	items, err := batch.GenerateReview(now.Add(7 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SegmentID != replacement.SegmentID {
		t.Fatalf("unexpected review sample: %#v", items)
	}
	if err := batch.DecideReview(items[0].ReviewItemID, "operator-a", "not independent", "passed", now); err == nil {
		t.Fatal("same operator should not review")
	}
	if err := batch.AssignReview(items[0].ReviewItemID, "reviewer-b", "supervisor", "独立复核分派", now.Add(time.Hour), now.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := batch.DecideReview(items[0].ReviewItemID, "reviewer-b", "摘要与证据一致", "passed", now.Add(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	manifest, err := batch.Seal("reviewer-b", now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if batch.State != StateApproved {
		t.Fatalf("unexpected state %s", batch.State)
	}
	_, valid, err := VerifyManifest(*manifest)
	if err != nil || !valid {
		t.Fatalf("manifest invalid: %v", err)
	}
	if err := ValidateAggregate(batch); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSampleIsDeterministic(t *testing.T) {
	now := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	build := func(order []string) []string {
		batch, _ := NewBatch("BATCH-2", "operator", now)
		baseline := testBaseline(now)
		baseline.SampleSize = 2
		_ = batch.Freeze(baseline, now)
		for _, id := range order {
			segment := testSegment(id, now, SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 12})
			_ = batch.AddSegment(segment, now)
			_, _ = AssessSegment(batch, id, now)
		}
		items, err := batch.GenerateReview(now)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.SegmentID
		}
		return ids
	}
	first := build([]string{"SEG-1", "SEG-2", "SEG-3"})
	second := build([]string{"SEG-3", "SEG-1", "SEG-2"})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("sample changed with insertion order: %v != %v", first, second)
	}
}

func TestCoverageDoesNotDoubleCountOverlap(t *testing.T) {
	now := time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)
	batch, _ := NewBatch("BATCH-3", "operator", now)
	_ = batch.Freeze(testBaseline(now), now)
	a := testSegment("SEG-A", now, SegmentMetrics{})
	a.Status = SegmentPassed
	a.BatchID = batch.BatchID
	b := testSegment("SEG-B", now.Add(10*time.Minute), SegmentMetrics{})
	b.EndTime = now.Add(40 * time.Minute)
	b.Status = SegmentPassed
	b.BatchID = batch.BatchID
	batch.Segments[a.SegmentID] = &a
	batch.Segments[b.SegmentID] = &b
	report := BuildCoverageReport(batch)
	if report.EffectiveSeconds != 2400 {
		t.Fatalf("expected 2400 seconds, got %v", report.EffectiveSeconds)
	}
	if len(report.Intervals) != 1 {
		t.Fatalf("expected merged interval, got %d", len(report.Intervals))
	}
}

func TestReplacementIssueWaitsForCoverageRecovery(t *testing.T) {
	now := time.Date(2026, 8, 27, 5, 0, 0, 0, time.UTC)
	batch, _ := NewBatch("BATCH-4", "operator", now)
	baseline := testBaseline(now)
	baseline.MinimumValidDuration = 1500
	_ = batch.Freeze(baseline, now)
	failed := testSegment("SEG-FAILED", now, SegmentMetrics{RFIOccupancy: .8})
	failed.EndTime = now.Add(10 * time.Minute)
	failed.ValidDurationSeconds = 500
	_ = batch.AddSegment(failed, now)
	_, _ = AssessSegment(batch, failed.SegmentID, now)
	_ = batch.Quarantine(failed.SegmentID, "窄带干扰", "补观", now)
	replacement := testSegment("SEG-REPLACEMENT", now, SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 12})
	replacement.EndTime = now.Add(10 * time.Minute)
	replacement.ValidDurationSeconds = 500
	replacement.ReplacesSegmentID = failed.SegmentID
	_ = batch.AddSegment(replacement, now)
	_, _ = AssessSegment(batch, replacement.SegmentID, now)
	if !batch.Issues[failed.SegmentID].Open {
		t.Fatal("issue closed before coverage recovered")
	}
	supplement := testSegment("SEG-SUPPLEMENT", now.Add(10*time.Minute), SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 12})
	_ = batch.AddSegment(supplement, now)
	_, _ = AssessSegment(batch, supplement.SegmentID, now)
	if batch.Issues[failed.SegmentID].Open {
		t.Fatal("issue remained open after replacement passed and coverage recovered")
	}
}

func TestReplacementPreviewDoesNotMutateBatch(t *testing.T) {
	now := time.Date(2026, 8, 27, 6, 0, 0, 0, time.UTC)
	batch, _ := NewBatch("BATCH-PREVIEW", "operator", now)
	baseline := testBaseline(now)
	baseline.MinimumValidDuration = 600
	_ = batch.Freeze(baseline, now)
	failed := testSegment("SEG-OLD", now, SegmentMetrics{RFIOccupancy: .8})
	failed.EndTime = now.Add(10 * time.Minute)
	failed.ValidDurationSeconds = 500
	_ = batch.AddSegment(failed, now)
	_, _ = AssessSegment(batch, failed.SegmentID, now)
	_ = batch.Quarantine(failed.SegmentID, "干扰", "补观", now)
	revision := batch.Revision
	count := len(batch.Segments)
	candidate := testSegment("SEG-NEW", now, SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 12})
	candidate.EndTime = now.Add(10 * time.Minute)
	candidate.ValidDurationSeconds = 500
	result, err := PreviewReplacement(batch, "issue-SEG-OLD", candidate, now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanCloseIssue || !result.CoverageSufficient || result.Decision != "passed" {
		t.Fatalf("unexpected preview: %#v", result)
	}
	if batch.Revision != revision || len(batch.Segments) != count {
		t.Fatal("preview mutated aggregate")
	}
}

func TestAssignedReviewerIsRequired(t *testing.T) {
	now := time.Date(2026, 8, 27, 7, 0, 0, 0, time.UTC)
	batch, _ := NewBatch("BATCH-ASSIGN", "operator-a", now)
	baseline := testBaseline(now)
	baseline.MinimumValidDuration = 600
	_ = batch.Freeze(baseline, now)
	segment := testSegment("SEG-REVIEW", now, SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 12})
	segment.EndTime = now.Add(10 * time.Minute)
	segment.ValidDurationSeconds = 500
	_ = batch.AddSegment(segment, now)
	_, _ = AssessSegment(batch, segment.SegmentID, now)
	items, err := batch.GenerateReview(now)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.AssignReview(items[0].ReviewItemID, "operator-a", "supervisor", "错误分派", now.Add(time.Hour), now); err == nil {
		t.Fatal("submitter assignment accepted")
	}
	if err := batch.AssignReview(items[0].ReviewItemID, "reviewer-a", "supervisor", "独立分派", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if err := batch.DecideReview(items[0].ReviewItemID, "reviewer-b", "证据一致", "passed", now); err == nil {
		t.Fatal("unassigned reviewer accepted")
	}
	if err := batch.DecideReview(items[0].ReviewItemID, "reviewer-a", "证据一致", "passed", now); err != nil {
		t.Fatal(err)
	}
}
