package valid_duration_coverage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"radio-observation-release-gate/internal/domain"
)

func TestValidDurationCoverage(t *testing.T) {
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch("B-VALID-DURATION", "operator", now)
	if err != nil {
		t.Fatal(err)
	}
	baseline := domain.Baseline{TelescopeID: "RT", TargetSource: "M87", FrequencyBand: "L", FrequencyLowHz: 1e9, FrequencyHighHz: 2e9, PlannedWindow: domain.PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 300, QualityThresholds: domain.QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .9, MaxPacketLoss: .1, MinSignalToNoise: 5}, SamplingSeed: "seed", SampleSize: 1}
	if err := batch.Freeze(baseline, now); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("mostly-invalid-observation"))
	segment := domain.ObservationSegment{SegmentID: "SEG-SPARSE", StartTime: now, EndTime: now.Add(10 * time.Minute), FrequencyLowHz: 1.1e9, FrequencyHighHz: 1.9e9, ValidDurationSeconds: 60, FileReference: "local://sparse", ContentSHA256: hex.EncodeToString(sum[:]), SubmittedBy: "operator", Metrics: domain.SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 10}}
	if err := batch.AddSegment(segment, now); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.AssessSegment(batch, segment.SegmentID, now); err != nil {
		t.Fatal(err)
	}
	report := domain.EvaluateEligibility(batch)
	if report.Coverage.EffectiveSeconds > segment.ValidDurationSeconds || report.CanGenerateReview {
		t.Fatalf("TestValidDurationCoverage: %v valid seconds produced %v effective seconds and review=%v", segment.ValidDurationSeconds, report.Coverage.EffectiveSeconds, report.CanGenerateReview)
	}
}
