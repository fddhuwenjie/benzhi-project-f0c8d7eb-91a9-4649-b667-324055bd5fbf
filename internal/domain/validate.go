package domain

import (
	"encoding/hex"
	"strings"
)

func ValidateBaseline(v Baseline) error {
	if strings.TrimSpace(v.TelescopeID) == "" || strings.TrimSpace(v.TargetSource) == "" {
		return rule("baseline_identity_required", "望远镜编号和目标源不能为空")
	}
	if strings.TrimSpace(v.FrequencyBand) == "" || v.FrequencyLowHz <= 0 || v.FrequencyHighHz <= v.FrequencyLowHz {
		return rule("baseline_frequency_invalid", "基线频段范围不合法")
	}
	if v.PlannedWindow.Start.IsZero() || !v.PlannedWindow.End.After(v.PlannedWindow.Start) {
		return rule("baseline_window_invalid", "计划时窗不合法")
	}
	if v.MinimumValidDuration <= 0 || v.MinimumValidDuration > v.PlannedWindow.End.Sub(v.PlannedWindow.Start).Seconds() {
		return rule("baseline_duration_invalid", "最小有效时长必须位于计划时窗内")
	}
	q := v.QualityThresholds
	if q.MaxRFIOccupancy < 0 || q.MaxRFIOccupancy > 1 || q.MinCompleteness < 0 || q.MinCompleteness > 1 || q.MaxPacketLoss < 0 || q.MaxPacketLoss > 1 || q.MinSignalToNoise < 0 {
		return rule("quality_threshold_invalid", "质检阈值超出允许范围")
	}
	if strings.TrimSpace(v.SamplingSeed) == "" || v.SampleSize <= 0 {
		return rule("sampling_rule_invalid", "抽样种子和样本数必须有效")
	}
	return nil
}

func ValidateSegment(b *ObservationBatch, s ObservationSegment) error {
	return validateSegmentAgainst(b.Baseline, b.BatchID, b.Segments, s)
}

// validateSegmentAgainst validates s against a candidate segments map without
// touching the aggregate. It is used by bulk registration to keep the batch
// untouched until every segment has been validated.
func validateSegmentAgainst(baseline Baseline, batchID string, segments map[string]*ObservationSegment, s ObservationSegment) error {
	if !stableIDPattern.MatchString(s.SegmentID) || strings.TrimSpace(s.SubmittedBy) == "" {
		return rule("segment_identity_required", "数据段编号和录入人员不能为空")
	}
	if s.BatchID != "" && s.BatchID != batchID {
		return rule("cross_batch_reference", "数据段不能引用其他批次")
	}
	if !s.EndTime.After(s.StartTime) || s.StartTime.Before(baseline.PlannedWindow.Start) || s.EndTime.After(baseline.PlannedWindow.End) {
		return rule("segment_window_invalid", "采集时段必须位于冻结计划时窗内")
	}
	if s.FrequencyLowHz < baseline.FrequencyLowHz || s.FrequencyHighHz > baseline.FrequencyHighHz || s.FrequencyHighHz <= s.FrequencyLowHz {
		return rule("segment_frequency_invalid", "数据段频率必须位于冻结频段内")
	}
	if s.ValidDurationSeconds <= 0 || s.ValidDurationSeconds > s.EndTime.Sub(s.StartTime).Seconds() {
		return rule("segment_duration_invalid", "数据段有效时长不合法")
	}
	if strings.TrimSpace(s.FileReference) == "" || !validSHA256(s.ContentSHA256) {
		return rule("segment_evidence_invalid", "文件引用不能为空且内容摘要必须是 SHA-256")
	}
	for _, existing := range segments {
		if strings.EqualFold(existing.ContentSHA256, s.ContentSHA256) {
			return rule("duplicate_content_digest", "内容摘要已在当前批次登记")
		}
		if existing.FileReference == s.FileReference {
			return rule("duplicate_file_reference", "文件引用已在当前批次登记")
		}
	}
	if s.ReplacesSegmentID != "" {
		old, ok := segments[s.ReplacesSegmentID]
		if !ok || old.Status != SegmentQuarantined {
			return rule("replacement_target_invalid", "替换目标必须是当前批次已隔离的数据段")
		}
	}
	return validateMetrics(s.Metrics)
}

func validateMetrics(m SegmentMetrics) error {
	if m.RFIOccupancy < 0 || m.RFIOccupancy > 1 || m.Completeness < 0 || m.Completeness > 1 || m.PacketLoss < 0 || m.PacketLoss > 1 || m.SignalToNoise < 0 {
		return rule("segment_metrics_invalid", "数据段测量值超出允许范围")
	}
	return nil
}

func validSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
