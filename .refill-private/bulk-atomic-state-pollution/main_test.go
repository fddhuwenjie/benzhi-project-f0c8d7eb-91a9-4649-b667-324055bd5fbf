package bulk_atomic_state_pollution_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"radio-observation-release-gate/internal/domain"
)

func bulkSegment(id string, now time.Time) domain.ObservationSegment {
	sum := sha256.Sum256([]byte(id))
	return domain.ObservationSegment{SegmentID: id, StartTime: now, EndTime: now.Add(10 * time.Minute), FrequencyLowHz: 1.1e9, FrequencyHighHz: 1.9e9, ValidDurationSeconds: 600, FileReference: "local://" + id, ContentSHA256: hex.EncodeToString(sum[:]), SubmittedBy: "operator", Metrics: domain.SegmentMetrics{RFIOccupancy: .1, Completeness: .99, PacketLoss: .01, SignalToNoise: 10}}
}

func TestBulkAtomicStatePollution(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch("B-BULK-ATOMIC", "operator", now)
	if err != nil {
		t.Fatal(err)
	}
	baseline := domain.Baseline{TelescopeID: "RT", TargetSource: "M87", FrequencyBand: "L", FrequencyLowHz: 1e9, FrequencyHighHz: 2e9, PlannedWindow: domain.PlannedWindow{Start: now, End: now.Add(time.Hour)}, MinimumValidDuration: 600, QualityThresholds: domain.QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .9, MaxPacketLoss: .1, MinSignalToNoise: 5}, SamplingSeed: "seed", SampleSize: 1}
	if err := batch.Freeze(baseline, now); err != nil {
		t.Fatal(err)
	}
	revision := batch.Revision
	valid := bulkSegment("SEG-VALID", now)
	invalid := bulkSegment("SEG-INVALID", now)
	invalid.EndTime = now.Add(2 * time.Hour)
	if _, err := batch.AddSegmentsAtomic([]domain.ObservationSegment{valid, invalid}, now); err == nil {
		t.Fatal("expected invalid bulk item to be rejected")
	}
	if len(batch.Segments) != 0 || batch.Revision != revision {
		t.Fatalf("TestBulkAtomicStatePollution: failed atomic bulk operation retained %d segment(s) at revision %d", len(batch.Segments), batch.Revision)
	}
}
