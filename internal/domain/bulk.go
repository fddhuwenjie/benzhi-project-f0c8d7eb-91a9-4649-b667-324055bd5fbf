package domain

import (
	"fmt"
	"strings"
	"time"
)

type BatchQualitySummary struct {
	Total                    int     `json:"total"`
	Passed                   int     `json:"passed"`
	Failed                   int     `json:"failed"`
	Skipped                  int     `json:"skipped"`
	EffectiveCoverageSeconds float64 `json:"effective_coverage_seconds"`
	CoverageGapSeconds       float64 `json:"coverage_gap_seconds"`
}

func (b *ObservationBatch) AddSegmentsAtomic(segments []ObservationSegment, now time.Time) ([]string, error) {
	if b.Terminal() {
		return nil, ErrTerminal
	}
	if len(segments) == 0 {
		return nil, rule("segments_empty", "至少需要登记一个数据段")
	}
	if b.State != StateFrozen && b.State != StateRemediation && b.State != StateQuality {
		return nil, rule("segment_registration_locked", "当前状态不允许登记数据段")
	}
	ids := make([]string, 0, len(segments))
	for i, seg := range segments {
		if strings.TrimSpace(seg.SegmentID) == "" {
			return nil, fmt.Errorf("第 %d 项数据段编号不能为空", i+1)
		}
		if _, exists := b.Segments[seg.SegmentID]; exists {
			return nil, fmt.Errorf("第 %d 项数据段 %s: %w", i+1, seg.SegmentID, rule("segment_id_exists", "数据段编号已存在"))
		}
		if err := ValidateSegment(b, seg); err != nil {
			return nil, fmt.Errorf("第 %d 项数据段 %s: %w", i+1, seg.SegmentID, err)
		}
		seg.BatchID = b.BatchID
		seg.Status = SegmentRegistered
		seg.RegisteredAt = now.UTC()
		b.Segments[seg.SegmentID] = &seg
		ids = append(ids, seg.SegmentID)
	}
	b.touch(now)
	return ids, nil
}

func AssessRegistered(b *ObservationBatch, now time.Time) ([]*QualityAssessment, BatchQualitySummary, error) {
	if b.Terminal() {
		return nil, BatchQualitySummary{}, ErrTerminal
	}
	if b.State != StateFrozen && b.State != StateQuality && b.State != StateRemediation {
		return nil, BatchQualitySummary{}, ErrInvalidState
	}
	ids := make([]string, 0)
	for id, seg := range b.Segments {
		if seg.Status == SegmentRegistered {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, BatchQualitySummary{}, rule("no_registered_segments", "当前没有待质检数据段")
	}
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	assessments := make([]*QualityAssessment, 0, len(ids))
	for _, id := range ids {
		a, err := assessOne(b, id, now)
		if err != nil {
			return nil, BatchQualitySummary{}, err
		}
		assessments = append(assessments, a)
	}
	if b.EffectiveCoverage() >= b.Baseline.MinimumValidDuration {
		for _, issue := range b.Issues {
			replacement := b.Segments[issue.ReplacementID]
			if issue.Open && replacement != nil && replacement.Status == SegmentPassed {
				t := now.UTC()
				issue.Open = false
				issue.ClosedAt = &t
			}
		}
	}
	b.State = StateQuality
	b.UpdatedAt = now.UTC()
	b.Revision++
	summary := BatchQualitySummary{Total: len(assessments), EffectiveCoverageSeconds: b.EffectiveCoverage()}
	for _, a := range assessments {
		if a.Decision == "passed" {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}
	for _, gap := range BuildCoverageReport(b).Gaps {
		summary.CoverageGapSeconds += gap.DurationSeconds
	}
	return assessments, summary, nil
}

func assessOne(b *ObservationBatch, segmentID string, now time.Time) (*QualityAssessment, error) {
	s, ok := b.Segments[segmentID]
	if !ok {
		return nil, ErrNotFound
	}
	q := b.Baseline.QualityThresholds
	results := []ThresholdResult{{Metric: "rfi_occupancy", Measured: s.Metrics.RFIOccupancy, Operator: "<=", Threshold: q.MaxRFIOccupancy, Passed: s.Metrics.RFIOccupancy <= q.MaxRFIOccupancy}, {Metric: "completeness", Measured: s.Metrics.Completeness, Operator: ">=", Threshold: q.MinCompleteness, Passed: s.Metrics.Completeness >= q.MinCompleteness}, {Metric: "packet_loss", Measured: s.Metrics.PacketLoss, Operator: "<=", Threshold: q.MaxPacketLoss, Passed: s.Metrics.PacketLoss <= q.MaxPacketLoss}, {Metric: "signal_to_noise", Measured: s.Metrics.SignalToNoise, Operator: ">=", Threshold: q.MinSignalToNoise, Passed: s.Metrics.SignalToNoise >= q.MinSignalToNoise}}
	codes := []string{}
	names := []string{"RFI_OCCUPANCY_HIGH", "COMPLETENESS_LOW", "PACKET_LOSS_HIGH", "SIGNAL_TO_NOISE_LOW"}
	for i, r := range results {
		if !r.Passed {
			codes = append(codes, names[i])
		}
	}
	decision := "passed"
	if len(codes) > 0 {
		decision = "failed"
		s.Status = SegmentFailed
		if b.Issues[segmentID] == nil {
			b.Issues[segmentID] = &QualityIssue{IssueID: "issue-" + segmentID, SegmentID: segmentID, Open: true, CreatedAt: now.UTC()}
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
	return a, nil
}

type ReplacementPreview struct {
	IssueID            string                        `json:"issue_id"`
	Candidate          ObservationSegment            `json:"candidate"`
	Decision           string                        `json:"decision"`
	IssueCodes         []string                      `json:"issue_codes"`
	ThresholdResults   []ThresholdResult             `json:"threshold_results"`
	Coverage           CoverageReport                `json:"coverage"`
	CoverageSufficient bool                          `json:"coverage_sufficient"`
	CanCloseIssue      bool                          `json:"can_close_issue"`
	RemainingGaps      []CoverageGap                 `json:"remaining_gaps"`
	Blockers           []string                      `json:"blockers"`
	CoverageDifference ReplacementCoverageDifference `json:"coverage_difference"`
}

type ReplacementCoverageDifference struct {
	OriginalStart        time.Time `json:"original_start"`
	OriginalEnd          time.Time `json:"original_end"`
	CandidateStart       time.Time `json:"candidate_start"`
	CandidateEnd         time.Time `json:"candidate_end"`
	StartDeltaSeconds    float64   `json:"start_delta_seconds"`
	EndDeltaSeconds      float64   `json:"end_delta_seconds"`
	FrequencyLowDeltaHz  float64   `json:"frequency_low_delta_hz"`
	FrequencyHighDeltaHz float64   `json:"frequency_high_delta_hz"`
}

func PreviewReplacement(b *ObservationBatch, issueID string, candidate ObservationSegment, now time.Time) (ReplacementPreview, error) {
	if b.Terminal() {
		return ReplacementPreview{}, ErrTerminal
	}
	if b.State == StateReview {
		return ReplacementPreview{}, rule("review_started", "批次已进入抽审")
	}
	var issue *QualityIssue
	for _, x := range b.Issues {
		if x.IssueID == issueID {
			issue = x
			break
		}
	}
	if issue == nil {
		return ReplacementPreview{}, ErrNotFound
	}
	if !issue.Open {
		return ReplacementPreview{}, rule("issue_closed", "目标质检问题已经关闭")
	}
	old := b.Segments[issue.SegmentID]
	if old == nil || old.Status != SegmentQuarantined {
		return ReplacementPreview{}, rule("segment_not_quarantined", "目标数据段未处于隔离状态")
	}
	candidate.ReplacesSegmentID = issue.SegmentID
	if err := ValidateSegment(b, candidate); err != nil {
		return ReplacementPreview{}, err
	}
	q := b.Baseline.QualityThresholds
	results := []ThresholdResult{{Metric: "rfi_occupancy", Measured: candidate.Metrics.RFIOccupancy, Operator: "<=", Threshold: q.MaxRFIOccupancy, Passed: candidate.Metrics.RFIOccupancy <= q.MaxRFIOccupancy}, {Metric: "completeness", Measured: candidate.Metrics.Completeness, Operator: ">=", Threshold: q.MinCompleteness, Passed: candidate.Metrics.Completeness >= q.MinCompleteness}, {Metric: "packet_loss", Measured: candidate.Metrics.PacketLoss, Operator: "<=", Threshold: q.MaxPacketLoss, Passed: candidate.Metrics.PacketLoss <= q.MaxPacketLoss}, {Metric: "signal_to_noise", Measured: candidate.Metrics.SignalToNoise, Operator: ">=", Threshold: q.MinSignalToNoise, Passed: candidate.Metrics.SignalToNoise >= q.MinSignalToNoise}}
	codes := []string{}
	names := []string{"RFI_OCCUPANCY_HIGH", "COMPLETENESS_LOW", "PACKET_LOSS_HIGH", "SIGNAL_TO_NOISE_LOW"}
	for i, r := range results {
		if !r.Passed {
			codes = append(codes, names[i])
		}
	}
	decision := "passed"
	if len(codes) > 0 {
		decision = "failed"
	}
	clone := *b
	clone.Segments = map[string]*ObservationSegment{}
	for id, s := range b.Segments {
		cp := *s
		clone.Segments[id] = &cp
	}
	cp := candidate
	cp.Status = SegmentPassed
	cp.BatchID = b.BatchID
	clone.Segments[candidate.SegmentID] = &cp
	report := BuildCoverageReport(&clone)
	blockers := []string{}
	if decision != "passed" {
		blockers = append(blockers, "replacement_quality_failed")
	}
	if !report.Sufficient {
		blockers = append(blockers, "coverage_insufficient")
	}
	difference := ReplacementCoverageDifference{OriginalStart: old.StartTime, OriginalEnd: old.EndTime, CandidateStart: candidate.StartTime, CandidateEnd: candidate.EndTime, StartDeltaSeconds: candidate.StartTime.Sub(old.StartTime).Seconds(), EndDeltaSeconds: candidate.EndTime.Sub(old.EndTime).Seconds(), FrequencyLowDeltaHz: candidate.FrequencyLowHz - old.FrequencyLowHz, FrequencyHighDeltaHz: candidate.FrequencyHighHz - old.FrequencyHighHz}
	return ReplacementPreview{IssueID: issueID, Candidate: candidate, Decision: decision, IssueCodes: codes, ThresholdResults: results, Coverage: report, CoverageSufficient: report.Sufficient, CanCloseIssue: decision == "passed" && report.Sufficient, RemainingGaps: report.Gaps, Blockers: blockers, CoverageDifference: difference}, nil
}
