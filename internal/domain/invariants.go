package domain

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateAggregate rejects snapshots that are valid JSON but violate domain links.
func ValidateAggregate(b *ObservationBatch) error {
	if b == nil || strings.TrimSpace(b.BatchID) == "" || strings.TrimSpace(b.CreatedBy) == "" {
		return fmt.Errorf("批次身份字段缺失")
	}
	if b.Revision < 1 || b.CreatedAt.IsZero() || b.UpdatedAt.Before(b.CreatedAt) {
		return fmt.Errorf("批次修订或时间戳不合法")
	}
	if !knownState(b.State) {
		return fmt.Errorf("未知批次状态 %q", b.State)
	}
	if b.State != StateDraft {
		if err := ValidateBaseline(b.Baseline); err != nil {
			return fmt.Errorf("冻结基线损坏: %w", err)
		}
		if b.Baseline.Version != 1 || b.Baseline.FrozenAt == nil {
			return fmt.Errorf("冻结基线版本边界缺失")
		}
	}
	if b.Segments == nil || b.Assessments == nil || b.Issues == nil {
		return fmt.Errorf("聚合集合字段缺失")
	}
	digests, files := map[string]string{}, map[string]string{}
	for id, segment := range b.Segments {
		if segment == nil || id != segment.SegmentID || segment.BatchID != b.BatchID {
			return fmt.Errorf("数据段 %q 的聚合归属不一致", id)
		}
		if !knownSegmentStatus(segment.Status) {
			return fmt.Errorf("数据段 %s 状态未知", id)
		}
		digest := strings.ToLower(segment.ContentSHA256)
		if other, exists := digests[digest]; exists {
			return fmt.Errorf("数据段 %s 与 %s 的内容摘要重复", id, other)
		}
		if other, exists := files[segment.FileReference]; exists {
			return fmt.Errorf("数据段 %s 与 %s 的文件引用重复", id, other)
		}
		digests[digest], files[segment.FileReference] = id, id
		if segment.ReplacesSegmentID != "" {
			old, exists := b.Segments[segment.ReplacesSegmentID]
			if !exists || old.ReplacesSegmentID == segment.SegmentID {
				return fmt.Errorf("数据段 %s 的替换关系不合法", id)
			}
		}
	}
	for id, assessment := range b.Assessments {
		segment, exists := b.Segments[id]
		if assessment == nil || !exists || assessment.SegmentID != id || assessment.BatchID != b.BatchID {
			return fmt.Errorf("质检结果 %s 的引用不一致", id)
		}
		if assessment.Decision != "passed" && assessment.Decision != "failed" {
			return fmt.Errorf("质检结果 %s 的结论不合法", id)
		}
		if assessment.Decision == "failed" && segment.Status == SegmentPassed {
			return fmt.Errorf("数据段 %s 的状态与质检结论矛盾", id)
		}
	}
	for id, issue := range b.Issues {
		if issue == nil || issue.SegmentID != id || b.Segments[id] == nil {
			return fmt.Errorf("质检问题 %s 的引用不一致", id)
		}
		if !issue.Open && (issue.ClosedAt == nil || issue.ReplacementID == "") {
			return fmt.Errorf("质检问题 %s 缺少关闭证据", id)
		}
	}
	if err := validateReviewInvariants(b); err != nil {
		return err
	}
	if b.Terminal() {
		if b.Manifest == nil || b.Manifest.BatchID != b.BatchID || b.Manifest.TerminalDecision != string(b.State) {
			return fmt.Errorf("终态批次与发布清单不一致")
		}
		_, valid, err := VerifyManifest(*b.Manifest)
		if err != nil || !valid {
			return fmt.Errorf("发布清单规范摘要无效")
		}
	} else if b.Manifest != nil {
		return fmt.Errorf("非终态批次不能包含发布清单")
	}
	return nil
}

func validateReviewInvariants(b *ObservationBatch) error {
	seen := map[string]bool{}
	ranks := make([]int, 0, len(b.ReviewItems))
	for _, item := range b.ReviewItems {
		if item == nil || item.BatchID != b.BatchID || b.Segments[item.SegmentID] == nil || seen[item.ReviewItemID] {
			return fmt.Errorf("抽审项目引用或编号不一致")
		}
		seen[item.ReviewItemID] = true
		ranks = append(ranks, item.SampleRank)
		if item.Decision == "" {
			if item.ReviewedAt != nil {
				return fmt.Errorf("待审项目包含裁定字段")
			}
			if item.ReviewerID != "" && item.DueAt == nil {
				return fmt.Errorf("已分派抽审项目缺少期限")
			}
			continue
		}
		if (item.Decision != "passed" && item.Decision != "failed") || item.ReviewerID == "" || item.EvidenceNote == "" || item.ReviewedAt == nil {
			return fmt.Errorf("已裁定抽审项目字段不完整")
		}
		if b.Segments[item.SegmentID].SubmittedBy == item.ReviewerID {
			return fmt.Errorf("抽审项目违反独立复核约束")
		}
	}
	sort.Ints(ranks)
	for i, rank := range ranks {
		if rank != i+1 {
			return fmt.Errorf("抽审样本排序不连续")
		}
	}
	if b.State == StateReview && len(b.ReviewItems) == 0 {
		return fmt.Errorf("抽审状态缺少锁定清单")
	}
	return nil
}

func knownState(state BatchState) bool {
	switch state {
	case StateDraft, StateFrozen, StateQuality, StateRemediation, StateReview, StateApproved, StateRejected:
		return true
	default:
		return false
	}
}

func knownSegmentStatus(status SegmentStatus) bool {
	switch status {
	case SegmentRegistered, SegmentPassed, SegmentFailed, SegmentQuarantined, SegmentReplaced:
		return true
	default:
		return false
	}
}
