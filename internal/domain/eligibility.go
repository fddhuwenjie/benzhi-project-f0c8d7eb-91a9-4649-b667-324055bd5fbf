package domain

import "sort"

type EligibilityBlocker struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	SegmentID string `json:"segment_id,omitempty"`
	IssueID   string `json:"issue_id,omitempty"`
}

type EligibilityReport struct {
	CanRegisterSegments bool                 `json:"can_register_segments"`
	CanGenerateReview   bool                 `json:"can_generate_review"`
	CanSeal             bool                 `json:"can_seal"`
	ReadOnly            bool                 `json:"read_only"`
	Coverage            CoverageReport       `json:"coverage"`
	Blockers            []EligibilityBlocker `json:"blockers"`
}

func EvaluateEligibility(b *ObservationBatch) EligibilityReport {
	report := EligibilityReport{
		ReadOnly:            b.Terminal(),
		CanRegisterSegments: b.State == StateFrozen || b.State == StateQuality || b.State == StateRemediation,
		Coverage:            BuildCoverageReport(b),
		Blockers:            []EligibilityBlocker{},
	}
	if b.Terminal() {
		report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "terminal_read_only", Message: "批次已封存为不可变终态"})
		return report
	}
	if b.State == StateDraft {
		report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "baseline_not_frozen", Message: "观测基线尚未冻结"})
		return report
	}
	activeSegments := 0
	for _, segment := range b.Segments {
		switch segment.Status {
		case SegmentRegistered:
			activeSegments++
			report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "segment_not_assessed", Message: "数据段尚未执行质检", SegmentID: segment.SegmentID})
		case SegmentFailed:
			activeSegments++
			report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "segment_quality_failed", Message: "数据段质检未通过，必须隔离并补观", SegmentID: segment.SegmentID})
		case SegmentQuarantined:
			report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "replacement_required", Message: "隔离数据段尚无合格替换段", SegmentID: segment.SegmentID})
		case SegmentPassed:
			activeSegments++
		}
	}
	if activeSegments == 0 {
		report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "qualified_segments_required", Message: "至少需要一个合格数据段"})
	}
	for _, issue := range b.Issues {
		if issue.Open {
			report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "open_quality_issue", Message: "质检问题尚未关闭", SegmentID: issue.SegmentID, IssueID: issue.IssueID})
		}
	}
	if !report.Coverage.Sufficient {
		report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "coverage_insufficient", Message: "合格数据段有效覆盖未达到冻结基线"})
	}
	if b.State == StateReview {
		for _, item := range b.ReviewItems {
			if item.Decision == "" {
				report.Blockers = append(report.Blockers, EligibilityBlocker{Code: "review_item_pending", Message: "抽审项目尚未裁定", SegmentID: item.SegmentID})
			}
		}
		report.CanSeal = b.ReviewComplete()
	} else {
		report.CanGenerateReview = len(report.Blockers) == 0 && (b.State == StateQuality || b.State == StateRemediation)
	}
	sort.Slice(report.Blockers, func(i, j int) bool {
		if report.Blockers[i].Code == report.Blockers[j].Code {
			return report.Blockers[i].SegmentID < report.Blockers[j].SegmentID
		}
		return report.Blockers[i].Code < report.Blockers[j].Code
	})
	return report
}
