package domain

import "time"

func AssessSegment(b *ObservationBatch, segmentID string, now time.Time) (*QualityAssessment, error) {
	if b.Terminal() {
		return nil, ErrTerminal
	}
	if b.State != StateFrozen && b.State != StateQuality && b.State != StateRemediation {
		return nil, ErrInvalidState
	}
	s, ok := b.Segments[segmentID]
	if !ok {
		return nil, ErrNotFound
	}
	if s.Status == SegmentQuarantined || s.Status == SegmentReplaced {
		return nil, rule("segment_not_assessable", "隔离或已替换数据段不能再次质检")
	}
	q := b.Baseline.QualityThresholds
	results := []ThresholdResult{
		{Metric: "rfi_occupancy", Measured: s.Metrics.RFIOccupancy, Operator: "<=", Threshold: q.MaxRFIOccupancy, Passed: s.Metrics.RFIOccupancy <= q.MaxRFIOccupancy},
		{Metric: "completeness", Measured: s.Metrics.Completeness, Operator: ">=", Threshold: q.MinCompleteness, Passed: s.Metrics.Completeness >= q.MinCompleteness},
		{Metric: "packet_loss", Measured: s.Metrics.PacketLoss, Operator: "<=", Threshold: q.MaxPacketLoss, Passed: s.Metrics.PacketLoss <= q.MaxPacketLoss},
		{Metric: "signal_to_noise", Measured: s.Metrics.SignalToNoise, Operator: ">=", Threshold: q.MinSignalToNoise, Passed: s.Metrics.SignalToNoise >= q.MinSignalToNoise},
	}
	codes := make([]string, 0)
	codeMap := []string{"RFI_OCCUPANCY_HIGH", "COMPLETENESS_LOW", "PACKET_LOSS_HIGH", "SIGNAL_TO_NOISE_LOW"}
	for i, r := range results {
		if !r.Passed {
			codes = append(codes, codeMap[i])
		}
	}
	decision := "passed"
	if len(codes) > 0 {
		decision = "failed"
		s.Status = SegmentFailed
		if _, exists := b.Issues[segmentID]; !exists {
			b.Issues[segmentID] = &QualityIssue{
				IssueID:   "issue-" + segmentID,
				SegmentID: segmentID,
				Open:      true,
				CreatedAt: now.UTC(),
			}
		}
	} else {
		s.Status = SegmentPassed
	}
	a := &QualityAssessment{AssessmentID: "qa-" + segmentID, BatchID: b.BatchID, SegmentID: segmentID, BaselineRevision: b.Baseline.Version, Metrics: s.Metrics, IssueCodes: codes, ThresholdResults: results, Decision: decision, EvaluatedAt: now.UTC()}
	b.Assessments[segmentID] = a
	if decision == "passed" && s.ReplacesSegmentID != "" {
		old := b.Segments[s.ReplacesSegmentID]
		old.Status = SegmentReplaced
		if issue := b.Issues[s.ReplacesSegmentID]; issue != nil {
			issue.ReplacementID = segmentID
		}
	}
	if decision == "passed" && b.EffectiveCoverage() >= b.Baseline.MinimumValidDuration {
		for _, issue := range b.Issues {
			replacement := b.Segments[issue.ReplacementID]
			if issue.Open && replacement != nil && replacement.Status == SegmentPassed {
				t := now.UTC()
				issue.Open = false
				issue.ClosedAt = &t
			}
		}
	}
	if decision == "failed" {
		b.State = StateQuality
	} else if b.AllCurrentSegmentsAssessed() {
		b.State = StateQuality
	}
	b.touch(now)
	return a, nil
}

func (b *ObservationBatch) AllCurrentSegmentsAssessed() bool {
	active := 0
	for _, s := range b.Segments {
		if s.Status == SegmentReplaced || s.Status == SegmentQuarantined {
			continue
		}
		active++
		if s.Status != SegmentPassed && s.Status != SegmentFailed {
			return false
		}
	}
	return active > 0
}

func (b *ObservationBatch) EffectiveCoverage() float64 {
	return BuildCoverageReport(b).EffectiveSeconds
}

func (b *ObservationBatch) ReadyForReview() error {
	if b.Terminal() {
		return ErrTerminal
	}
	if b.State != StateQuality && b.State != StateRemediation {
		return ErrInvalidState
	}
	report := EvaluateEligibility(b)
	if !report.CanGenerateReview {
		if len(report.Blockers) > 0 {
			return rule(report.Blockers[0].Code, report.Blockers[0].Message)
		}
		return rule("review_not_ready", "批次尚未取得抽审资格")
	}
	return nil
}
