package domain

import (
	"regexp"
	"strings"
	"time"
)

var stableIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func NewBatch(id, creator string, now time.Time) (*ObservationBatch, error) {
	if !stableIDPattern.MatchString(id) || strings.TrimSpace(creator) == "" {
		return nil, rule("batch_identity_required", "批次编号和创建人不能为空")
	}
	return &ObservationBatch{
		BatchID: id, State: StateDraft, Revision: 1, CreatedBy: creator,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
		Segments: map[string]*ObservationSegment{}, Assessments: map[string]*QualityAssessment{}, Issues: map[string]*QualityIssue{},
	}, nil
}

func (b *ObservationBatch) Freeze(v Baseline, now time.Time) error {
	if b.Terminal() {
		return ErrTerminal
	}
	if b.State != StateDraft {
		return rule("baseline_already_frozen", "基线已冻结，禁止原位修改")
	}
	if err := ValidateBaseline(v); err != nil {
		return err
	}
	v.Version = 1
	t := now.UTC()
	v.FrozenAt = &t
	b.Baseline = v
	b.State = StateFrozen
	b.touch(now)
	return nil
}

func (b *ObservationBatch) AddSegment(s ObservationSegment, now time.Time) error {
	if b.Terminal() {
		return ErrTerminal
	}
	if b.State != StateFrozen && b.State != StateRemediation && b.State != StateQuality {
		return rule("segment_registration_locked", "当前状态不允许登记数据段")
	}
	if _, exists := b.Segments[s.SegmentID]; exists {
		return rule("segment_id_exists", "数据段编号已存在")
	}
	if err := ValidateSegment(b, s); err != nil {
		return err
	}
	s.BatchID = b.BatchID
	s.Status = SegmentRegistered
	s.RegisteredAt = now.UTC()
	b.Segments[s.SegmentID] = &s
	b.touch(now)
	return nil
}

func (b *ObservationBatch) Quarantine(segmentID, reason, plan string, now time.Time) error {
	if b.Terminal() {
		return ErrTerminal
	}
	seg, ok := b.Segments[segmentID]
	if !ok {
		return ErrNotFound
	}
	if seg.Status != SegmentFailed {
		return rule("segment_not_failed", "只有质检失败的数据段可以隔离")
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(plan) == "" {
		return rule("remediation_required", "隔离原因和补观计划不能为空")
	}
	seg.Status = SegmentQuarantined
	issue := b.Issues[segmentID]
	if issue == nil {
		issue = &QualityIssue{IssueID: "issue-" + segmentID, SegmentID: segmentID, Open: true, CreatedAt: now.UTC()}
		b.Issues[segmentID] = issue
	}
	issue.Reason = reason
	issue.ReobservationPlan = plan
	b.State = StateRemediation
	b.touch(now)
	return nil
}

func (b *ObservationBatch) touch(now time.Time) {
	b.Revision++
	b.UpdatedAt = now.UTC()
}
