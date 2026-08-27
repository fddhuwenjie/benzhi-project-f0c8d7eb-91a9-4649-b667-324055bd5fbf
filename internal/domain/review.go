package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

type rankedSegment struct {
	segment *ObservationSegment
	key     string
}

func (b *ObservationBatch) GenerateReview(now time.Time) ([]*ReviewItem, error) {
	if err := b.ReadyForReview(); err != nil {
		return nil, err
	}
	ranked := make([]rankedSegment, 0)
	for _, s := range b.Segments {
		if s.Status != SegmentPassed {
			continue
		}
		sum := sha256.Sum256([]byte(b.Baseline.SamplingSeed + "\x00" + s.SegmentID + "\x00" + s.ContentSHA256))
		ranked = append(ranked, rankedSegment{s, hex.EncodeToString(sum[:])})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].key == ranked[j].key {
			return ranked[i].segment.SegmentID < ranked[j].segment.SegmentID
		}
		return ranked[i].key < ranked[j].key
	})
	count := b.Baseline.SampleSize
	if count > len(ranked) {
		count = len(ranked)
	}
	lockedRevision := b.Revision + 1
	b.ReviewItems = make([]*ReviewItem, 0, count)
	for i := 0; i < count; i++ {
		b.ReviewItems = append(b.ReviewItems, &ReviewItem{ReviewItemID: "review-" + ranked[i].segment.SegmentID, BatchID: b.BatchID, SegmentID: ranked[i].segment.SegmentID, SampleRank: i + 1, LockedRevision: lockedRevision})
	}
	b.State = StateReview
	b.touch(now)
	return b.ReviewItems, nil
}

func (b *ObservationBatch) DecideReview(itemID, reviewer, note, decision string, now time.Time) error {
	if b.Terminal() {
		return ErrTerminal
	}
	if b.State != StateReview {
		return ErrInvalidState
	}
	if decision != "passed" && decision != "failed" {
		return rule("review_decision_invalid", "抽审结论必须为 passed 或 failed")
	}
	if strings.TrimSpace(reviewer) == "" || strings.TrimSpace(note) == "" {
		return rule("review_evidence_required", "复核员和证据核对说明不能为空")
	}
	var target *ReviewItem
	for _, item := range b.ReviewItems {
		if item.ReviewItemID == itemID {
			target = item
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}
	if target.ReviewerID == "" || target.ReviewerID != reviewer {
		return rule("reviewer_not_assigned", "只有被分派的复核员可以提交裁定")
	}
	if target.Decision != "" {
		return rule("review_already_decided", "抽审项目已经裁定")
	}
	seg := b.Segments[target.SegmentID]
	if seg.SubmittedBy == reviewer {
		return rule("reviewer_not_independent", "复核员必须不同于数据录入人员")
	}
	t := now.UTC()
	target.ReviewerID = reviewer
	target.EvidenceNote = note
	target.Decision = decision
	target.ReviewedAt = &t
	b.touch(now)
	return nil
}

func (b *ObservationBatch) AssignReview(itemID, reviewer, by, reason string, dueAt, now time.Time) error {
	if b.Terminal() {
		return ErrTerminal
	}
	if b.State != StateReview {
		return ErrInvalidState
	}
	if strings.TrimSpace(reviewer) == "" {
		return rule("reviewer_required", "复核员不能为空")
	}
	if strings.TrimSpace(by) == "" {
		return rule("actor_required", "操作人不能为空")
	}
	if strings.TrimSpace(reason) == "" {
		return rule("assignment_reason_required", "分派原因不能为空")
	}
	if !dueAt.After(now) {
		return rule("review_due_invalid", "计划完成时间必须晚于当前时间")
	}
	var target *ReviewItem
	for _, item := range b.ReviewItems {
		if item.ReviewItemID == itemID {
			target = item
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}
	if target.Decision != "" {
		return rule("review_already_decided", "抽审项目已经裁定")
	}
	seg := b.Segments[target.SegmentID]
	if seg != nil && seg.SubmittedBy == reviewer {
		return rule("reviewer_not_independent", "复核员必须不同于数据录入人员")
	}
	from := target.ReviewerID
	t := now.UTC()
	target.ReviewerID = reviewer
	target.AssignedBy = by
	target.AssignmentReason = reason
	due := dueAt.UTC()
	target.DueAt = &due
	target.AssignmentHistory = append(target.AssignmentHistory, ReviewAssignment{At: t, By: by, From: from, To: reviewer, Reason: reason, DueAt: due})
	b.touch(now)
	return nil
}

func (b *ObservationBatch) ReviewComplete() bool {
	if len(b.ReviewItems) == 0 {
		return false
	}
	for _, item := range b.ReviewItems {
		if item.Decision == "" {
			return false
		}
	}
	return true
}

func (b *ObservationBatch) ReviewPassed() bool {
	if !b.ReviewComplete() {
		return false
	}
	for _, item := range b.ReviewItems {
		if item.Decision != "passed" {
			return false
		}
	}
	return true
}
