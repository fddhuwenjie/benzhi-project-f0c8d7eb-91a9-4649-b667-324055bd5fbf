package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"radio-observation-release-gate/internal/audit"
	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
)

type Service struct {
	repo  *storage.Repository
	clock func() time.Time
}

func New(repo *storage.Repository) *Service { return &Service{repo: repo, clock: time.Now} }

func fingerprint(operation string, command any) (string, error) {
	raw, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Command   any    `json:"command"`
	}{operation, command})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validateMeta(meta RequestMeta) error {
	if strings.TrimSpace(meta.RequestID) == "" {
		return &Error{Code: "request_id_required", Message: "request_id 不能为空", HTTPStatus: 400}
	}
	if strings.TrimSpace(meta.Actor) == "" {
		return &Error{Code: "actor_required", Message: "actor 不能为空", HTTPStatus: 400}
	}
	if meta.ExpectedRevision < 0 {
		return &Error{Code: "revision_invalid", Message: "expected_revision 不能为负数", HTTPStatus: 400}
	}
	return nil
}

func replay(record *storage.IdempotencyRecord, fp string) (*CommandResult, error) {
	if record.Fingerprint != fp {
		return nil, &Error{Code: "idempotency_mismatch", Message: "相同 request_id 的请求内容不一致", HTTPStatus: 409}
	}
	var result CommandResult
	if err := json.Unmarshal(record.Response, &result); err != nil {
		return nil, err
	}
	result.Replayed = true
	return &result, nil
}

func (s *Service) CreateBatch(cmd CreateBatchCommand) (*CommandResult, error) {
	if err := validateMeta(cmd.RequestMeta); err != nil {
		return nil, err
	}
	if cmd.ExpectedRevision != 0 {
		return nil, &Error{Code: "stale_revision", Message: "创建批次时 expected_revision 必须为 0", HTTPStatus: 409}
	}
	if strings.TrimSpace(cmd.BatchID) == "" {
		return nil, &Error{Code: "batch_id_required", Message: "batch_id 不能为空", HTTPStatus: 400}
	}
	unlock := s.repo.Lock(cmd.BatchID)
	defer unlock()
	fp, err := fingerprint("create_batch", cmd)
	if err != nil {
		return nil, err
	}
	if exists, _ := s.repo.Exists(cmd.BatchID); exists {
		record, loadErr := s.repo.Idempotency(cmd.BatchID, cmd.RequestID)
		if loadErr != nil {
			return nil, loadErr
		}
		if record != nil {
			return replay(record, fp)
		}
		return nil, &Error{Code: "batch_exists", Message: "批次编号已存在", HTTPStatus: 409}
	}
	batch, err := domain.NewBatch(cmd.BatchID, cmd.Actor, s.clock())
	if err != nil {
		return nil, mapError(err)
	}
	result := &CommandResult{Batch: batch}
	raw, _ := json.Marshal(result)
	record := &storage.IdempotencyRecord{RequestID: cmd.RequestID, Fingerprint: fp, Status: 200, Response: raw}
	if err := s.repo.Commit(batch, storage.Commit{EventType: "batch.created", Actor: cmd.Actor, Payload: cmd, Idempotency: record}); err != nil {
		return nil, err
	}
	return result, nil
}

type mutation func(*domain.ObservationBatch) error

type resultMutation func(*domain.ObservationBatch) (*CommandResult, error)

func (s *Service) mutateResult(batchID, operation string, meta RequestMeta, command any, fn resultMutation) (*CommandResult, error) {
	if err := validateMeta(meta); err != nil {
		return nil, err
	}
	if strings.TrimSpace(batchID) == "" {
		return nil, &Error{Code: "batch_id_required", Message: "batch_id 不能为空", HTTPStatus: 400}
	}
	unlock := s.repo.Lock(batchID)
	defer unlock()
	fp, err := fingerprint(operation, command)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.Idempotency(batchID, meta.RequestID)
	if err != nil {
		return nil, err
	}
	if record != nil {
		return replay(record, fp)
	}
	batch, err := s.repo.Load(batchID)
	if err != nil {
		return nil, mapError(err)
	}
	if batch.Revision != meta.ExpectedRevision {
		return nil, &Error{Code: "stale_revision", Message: fmt.Sprintf("页面修订 %d 已过期，当前修订为 %d", meta.ExpectedRevision, batch.Revision), CurrentRevision: batch.Revision, HTTPStatus: 409}
	}
	result, err := fn(batch)
	if err != nil {
		return nil, mapError(err)
	}
	if result == nil {
		result = &CommandResult{Batch: batch}
	}
	result.Batch = batch
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	idem := &storage.IdempotencyRecord{RequestID: meta.RequestID, Fingerprint: fp, Status: 200, Response: raw}
	payload := command
	if result.auditPayload != nil {
		payload = result.auditPayload
	}
	if err := s.repo.Commit(batch, storage.Commit{EventType: operation, Actor: meta.Actor, Payload: payload, Idempotency: idem}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) mutate(batchID, operation string, meta RequestMeta, command any, fn mutation) (*CommandResult, error) {
	if err := validateMeta(meta); err != nil {
		return nil, err
	}
	if strings.TrimSpace(batchID) == "" {
		return nil, &Error{Code: "batch_id_required", Message: "batch_id 不能为空", HTTPStatus: 400}
	}
	unlock := s.repo.Lock(batchID)
	defer unlock()
	fp, err := fingerprint(operation, command)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.Idempotency(batchID, meta.RequestID)
	if err != nil {
		return nil, err
	}
	if record != nil {
		return replay(record, fp)
	}
	batch, err := s.repo.Load(batchID)
	if err != nil {
		return nil, mapError(err)
	}
	if batch.Revision != meta.ExpectedRevision {
		return nil, &Error{Code: "stale_revision", Message: fmt.Sprintf("页面修订 %d 已过期，当前修订为 %d", meta.ExpectedRevision, batch.Revision), CurrentRevision: batch.Revision, HTTPStatus: 409}
	}
	if err := fn(batch); err != nil {
		return nil, mapError(err)
	}
	result := &CommandResult{Batch: batch}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	idem := &storage.IdempotencyRecord{RequestID: meta.RequestID, Fingerprint: fp, Status: 200, Response: raw}
	var manifest *domain.ReleaseManifest
	if operation == "batch.sealed" {
		manifest = batch.Manifest
	}
	if err := s.repo.Commit(batch, storage.Commit{EventType: operation, Actor: meta.Actor, Payload: command, Idempotency: idem, Manifest: manifest}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) FreezeBaseline(batchID string, cmd FreezeBaselineCommand) (*CommandResult, error) {
	return s.mutate(batchID, "baseline.frozen", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error { return b.Freeze(cmd.Baseline, s.clock()) })
}

func (s *Service) RegisterSegment(batchID string, cmd RegisterSegmentCommand) (*CommandResult, error) {
	unlockGlobal := s.repo.LockGlobal()
	defer unlockGlobal()
	if other, found, err := s.repo.FindContentDigest(cmd.Segment.ContentSHA256, batchID); err != nil {
		return nil, err
	} else if found {
		return nil, &Error{Code: "cross_batch_digest", Message: "内容摘要已由批次 " + other + " 登记", HTTPStatus: 409}
	}
	return s.mutate(batchID, "segment.registered", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error { return b.AddSegment(cmd.Segment, s.clock()) })
}

type BulkItemError struct {
	InputIndex int    `json:"input_index"`
	SegmentID  string `json:"segment_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (s *Service) RegisterSegments(batchID string, cmd RegisterSegmentsCommand) (*CommandResult, error) {
	if len(cmd.Segments) == 0 {
		return nil, &Error{Code: "segments_empty", Message: "至少需要登记一个数据段", HTTPStatus: 400}
	}
	if len(cmd.Segments) > 100 {
		return nil, &Error{Code: "segments_limit_exceeded", Message: "单次最多登记 100 个数据段", HTTPStatus: 400}
	}
	if err := validateMeta(cmd.RequestMeta); err != nil {
		return nil, err
	}
	unlockGlobal := s.repo.LockGlobal()
	defer unlockGlobal()
	unlock := s.repo.Lock(batchID)
	defer unlock()
	fp, err := fingerprint("segments.registered", cmd)
	if err != nil {
		return nil, err
	}
	if record, err := s.repo.Idempotency(batchID, cmd.RequestID); err != nil {
		return nil, err
	} else if record != nil {
		return replay(record, fp)
	}
	batch, err := s.repo.Load(batchID)
	if err != nil {
		return nil, mapError(err)
	}
	if batch.Revision != cmd.ExpectedRevision {
		return nil, &Error{Code: "stale_revision", Message: fmt.Sprintf("页面修订 %d 已过期，当前修订为 %d", cmd.ExpectedRevision, batch.Revision), CurrentRevision: batch.Revision, HTTPStatus: 409}
	}
	errs := make([]BulkItemError, 0)
	seenIDs := map[string]int{}
	seenFiles := map[string]int{}
	seenDigests := map[string]int{}
	seenTargets := map[string]int{}
	for i, seg := range cmd.Segments {
		code, msg := "", ""
		key := strings.ToLower(seg.ContentSHA256)
		if p, ok := seenIDs[seg.SegmentID]; ok {
			code = "duplicate_segment_id"
			msg = fmt.Sprintf("与第 %d 项数据段编号重复", p+1)
		} else {
			seenIDs[seg.SegmentID] = i
		}
		if p, ok := seenFiles[seg.FileReference]; ok && code == "" {
			code = "duplicate_file_reference"
			msg = fmt.Sprintf("与第 %d 项文件引用重复", p+1)
		} else {
			seenFiles[seg.FileReference] = i
		}
		if p, ok := seenDigests[key]; ok && code == "" {
			code = "duplicate_content_digest"
			msg = fmt.Sprintf("与第 %d 项内容摘要重复", p+1)
		} else {
			seenDigests[key] = i
		}
		if seg.ReplacesSegmentID != "" {
			if p, ok := seenTargets[seg.ReplacesSegmentID]; ok && code == "" {
				code = "replacement_conflict"
				msg = fmt.Sprintf("与第 %d 项替换同一隔离段", p+1)
			} else {
				seenTargets[seg.ReplacesSegmentID] = i
			}
		}
		if other, found, e := s.repo.FindContentDigest(seg.ContentSHA256, batchID); e != nil {
			return nil, e
		} else if found && code == "" {
			code = "cross_batch_digest"
			msg = "内容摘要已由批次 " + other + " 登记"
		}
		if code != "" {
			errs = append(errs, BulkItemError{InputIndex: i, SegmentID: seg.SegmentID, Code: code, Message: msg})
		}
	}
	if len(errs) == 0 {
		clone := *batch
		clone.Segments = map[string]*domain.ObservationSegment{}
		for id, seg := range batch.Segments {
			cp := *seg
			clone.Segments[id] = &cp
		}
		_, e := clone.AddSegmentsAtomic(cmd.Segments, s.clock())
		if e != nil {
			var rule *domain.RuleError
			if errors.As(e, &rule) {
				index := locateBulkError(e)
				segmentID := ""
				if index >= 0 && index < len(cmd.Segments) {
					segmentID = cmd.Segments[index].SegmentID
				}
				errs = append(errs, BulkItemError{InputIndex: index, SegmentID: segmentID, Code: rule.Code, Message: rule.Message})
			} else {
				return nil, mapError(e)
			}
		}
	}
	if len(errs) > 0 {
		return nil, &Error{Code: "bulk_validation_failed", Message: "批量登记校验失败", Details: errs, HTTPStatus: 422}
	}
	ids, err := batch.AddSegmentsAtomic(cmd.Segments, s.clock())
	if err != nil {
		return nil, mapError(err)
	}
	result := &CommandResult{Batch: batch, Registered: make([]RegisteredSegmentResult, len(ids))}
	for i, id := range ids {
		result.Registered[i] = RegisteredSegmentResult{InputIndex: i, SegmentID: id, Status: domain.SegmentRegistered}
	}
	raw, _ := json.Marshal(result)
	idem := &storage.IdempotencyRecord{RequestID: cmd.RequestID, Fingerprint: fp, Status: 200, Response: raw}
	if err := s.repo.Commit(batch, storage.Commit{EventType: "segments.registered", Actor: cmd.Actor, Payload: cmd, Idempotency: idem}); err != nil {
		return nil, err
	}
	return result, nil
}

func locateBulkError(err error) int {
	var index int
	_, _ = fmt.Sscanf(err.Error(), "第 %d 项", &index)
	if index > 0 {
		return index - 1
	}
	return 0
}

func (s *Service) AssessSegment(batchID string, cmd AssessSegmentCommand) (*CommandResult, error) {
	return s.mutate(batchID, "quality.assessed", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error {
		_, err := domain.AssessSegment(b, cmd.SegmentID, s.clock())
		return err
	})
}

func (s *Service) AssessBatch(batchID string, cmd AssessBatchCommand) (*CommandResult, error) {
	return s.mutateResult(batchID, "quality.batch_assessed", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) (*CommandResult, error) {
		items, summary, err := domain.AssessRegistered(b, s.clock())
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.SegmentID
		}
		auditPayload := struct {
			RequestMeta
			BaselineVersion int                        `json:"baseline_version"`
			SegmentIDs      []string                   `json:"segment_ids"`
			Summary         domain.BatchQualitySummary `json:"summary"`
		}{cmd.RequestMeta, b.Baseline.Version, ids, summary}
		return &CommandResult{Batch: b, Assessments: items, QualitySummary: &summary, auditPayload: auditPayload}, nil
	})
}

func (s *Service) PreviewReplacement(batchID string, cmd PreviewReplacementCommand) (domain.ReplacementPreview, error) {
	if cmd.ExpectedRevision < 0 {
		return domain.ReplacementPreview{}, &Error{Code: "revision_invalid", Message: "expected_revision 不能为负数", HTTPStatus: 400}
	}
	b, err := s.repo.Load(batchID)
	if err != nil {
		return domain.ReplacementPreview{}, mapError(err)
	}
	if b.Revision != cmd.ExpectedRevision {
		return domain.ReplacementPreview{}, &Error{Code: "stale_revision", Message: fmt.Sprintf("页面修订 %d 已过期，当前修订为 %d", cmd.ExpectedRevision, b.Revision), CurrentRevision: b.Revision, HTTPStatus: 409}
	}
	if other, found, err := s.repo.FindContentDigest(cmd.Segment.ContentSHA256, batchID); err != nil {
		return domain.ReplacementPreview{}, err
	} else if found {
		return domain.ReplacementPreview{}, &Error{Code: "cross_batch_digest", Message: "内容摘要已由批次 " + other + " 登记", HTTPStatus: 409}
	}
	result, err := domain.PreviewReplacement(b, cmd.IssueID, cmd.Segment, s.clock())
	if err != nil {
		return domain.ReplacementPreview{}, mapError(err)
	}
	return result, nil
}

func (s *Service) Quarantine(batchID string, cmd QuarantineCommand) (*CommandResult, error) {
	return s.mutate(batchID, "segment.quarantined", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error {
		return b.Quarantine(cmd.SegmentID, cmd.Reason, cmd.ReobservationPlan, s.clock())
	})
}

func (s *Service) GenerateReview(batchID string, cmd GenerateReviewCommand) (*CommandResult, error) {
	return s.mutate(batchID, "review.generated", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error { _, err := b.GenerateReview(s.clock()); return err })
}

func (s *Service) DecideReview(batchID string, cmd DecideReviewCommand) (*CommandResult, error) {
	return s.mutate(batchID, "review.decided", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error {
		return b.DecideReview(cmd.ReviewItemID, cmd.Actor, cmd.EvidenceNote, cmd.Decision, s.clock())
	})
}

func (s *Service) AssignReview(batchID string, cmd AssignReviewCommand) (*CommandResult, error) {
	return s.mutateResult(batchID, "review.assigned", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) (*CommandResult, error) {
		previous := ""
		for _, item := range b.ReviewItems {
			if item.ReviewItemID == cmd.ReviewItemID {
				previous = item.ReviewerID
				break
			}
		}
		if err := b.AssignReview(cmd.ReviewItemID, cmd.ReviewerID, cmd.Actor, cmd.Reason, cmd.DueAt, s.clock()); err != nil {
			return nil, err
		}
		payload := struct {
			RequestMeta
			ReviewItemID     string    `json:"review_item_id"`
			PreviousReviewer string    `json:"previous_reviewer,omitempty"`
			ReviewerID       string    `json:"reviewer_id"`
			DueAt            time.Time `json:"due_at"`
			Reason           string    `json:"reason"`
		}{cmd.RequestMeta, cmd.ReviewItemID, previous, cmd.ReviewerID, cmd.DueAt, cmd.Reason}
		return &CommandResult{Batch: b, auditPayload: payload}, nil
	})
}

func (s *Service) Seal(batchID string, cmd SealCommand) (*CommandResult, error) {
	return s.mutate(batchID, "batch.sealed", cmd.RequestMeta, cmd, func(b *domain.ObservationBatch) error {
		_, err := b.Seal(cmd.Actor, s.clock())
		return err
	})
}

func (s *Service) GetBatch(id string) (BatchView, error) {
	batch, err := s.repo.Load(id)
	if err != nil {
		return BatchView{}, mapError(err)
	}
	return BuildView(batch), nil
}

func (s *Service) ListBatches() ([]BatchView, error) {
	batches, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	views := make([]BatchView, 0, len(batches))
	for _, b := range batches {
		views = append(views, BuildView(b))
	}
	return views, nil
}

func (s *Service) QueryBatches(filter BatchListFilter) (BatchListResult, error) {
	batches, err := s.repo.List()
	if err != nil {
		return BatchListResult{}, err
	}
	items := make([]BatchListItem, 0)
	counts := map[domain.BatchState]int{domain.StateDraft: 0, domain.StateFrozen: 0, domain.StateQuality: 0, domain.StateRemediation: 0, domain.StateReview: 0, domain.StateApproved: 0, domain.StateRejected: 0}
	todoTotal := 0
	for _, b := range batches {
		if filter.BatchID != "" && !strings.Contains(strings.ToLower(b.BatchID), strings.ToLower(filter.BatchID)) {
			continue
		}
		if filter.TelescopeID != "" && !strings.Contains(strings.ToLower(b.Baseline.TelescopeID), strings.ToLower(filter.TelescopeID)) {
			continue
		}
		if filter.TargetSource != "" && !strings.Contains(strings.ToLower(b.Baseline.TargetSource), strings.ToLower(filter.TargetSource)) {
			continue
		}
		if filter.State != "" && b.State != filter.State {
			continue
		}
		v := BuildView(b)
		todo := v.OpenIssueCount + v.PendingReviewCount
		gap := 0.0
		for _, g := range v.Coverage.Gaps {
			gap += g.DurationSeconds
		}
		if filter.TodoOnly && todo == 0 && gap == 0 {
			continue
		}
		counts[b.State]++
		todoTotal += todo
		items = append(items, BatchListItem{BatchView: v, CoverageGapSeconds: gap, TerminalReadOnly: b.Terminal(), UpdatedAt: b.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].Batch.BatchID < items[j].Batch.BatchID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	total := len(items)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	pages := 0
	if total > 0 {
		pages = (total + filter.PageSize - 1) / filter.PageSize
	}
	return BatchListResult{Batches: items[start:end], StateCounts: counts, TodoTotal: todoTotal, Pagination: Pagination{Page: filter.Page, PageSize: filter.PageSize, TotalItems: total, TotalPages: pages}, Filter: BatchListFilterView{BatchID: filter.BatchID, TelescopeID: filter.TelescopeID, TargetSource: filter.TargetSource, State: filter.State, TodoOnly: filter.TodoOnly}}, nil
}

func (s *Service) Timeline(id string) (TimelineView, error) {
	events, err := s.repo.Events(id)
	if err != nil {
		return TimelineView{}, err
	}
	if len(events) == 0 {
		return TimelineView{}, mapError(storage.ErrNotFound)
	}
	integrity, err := audit.Inspect(events)
	if err != nil {
		return TimelineView{}, err
	}
	return TimelineView{Items: audit.ProjectTimeline(events), Integrity: integrity}, nil
}

func (s *Service) VerifyManifest(id string) (ManifestVerification, error) {
	m, err := s.repo.Manifest(id)
	if err != nil {
		return ManifestVerification{}, mapError(err)
	}
	recomputed, valid, err := domain.VerifyManifest(*m)
	if err != nil {
		return ManifestVerification{}, err
	}
	return ManifestVerification{BatchID: id, StoredSHA256: m.CanonicalSHA256, RecomputedSHA256: recomputed, Valid: valid}, nil
}

func (s *Service) Health() map[string]any {
	result, err := s.repo.VerifyAll()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	return map[string]any{"status": "ok", "batches": result.Batches, "events": result.Events}
}
