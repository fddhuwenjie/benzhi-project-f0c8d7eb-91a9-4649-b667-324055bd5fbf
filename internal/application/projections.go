package application

import (
	"sort"
	"time"

	"radio-observation-release-gate/internal/domain"
)

type SegmentProjection struct {
	SegmentID            string                   `json:"segment_id"`
	Status               domain.SegmentStatus     `json:"status"`
	SubmittedBy          string                   `json:"submitted_by"`
	RegisteredAt         time.Time                `json:"registered_at"`
	ValidDurationSeconds float64                  `json:"valid_duration_seconds"`
	FileReference        string                   `json:"file_reference"`
	ContentSHA256        string                   `json:"content_sha256"`
	ReplacesSegmentID    string                   `json:"replaces_segment_id,omitempty"`
	QualityDecision      string                   `json:"quality_decision,omitempty"`
	IssueCodes           []string                 `json:"issue_codes"`
	ThresholdResults     []domain.ThresholdResult `json:"threshold_results"`
}

type IssueProjection struct {
	IssueID           string               `json:"issue_id"`
	SegmentID         string               `json:"segment_id"`
	SegmentStatus     domain.SegmentStatus `json:"segment_status"`
	IssueCodes        []string             `json:"issue_codes"`
	Reason            string               `json:"reason,omitempty"`
	ReobservationPlan string               `json:"reobservation_plan,omitempty"`
	ReplacementID     string               `json:"replacement_id,omitempty"`
	Open              bool                 `json:"open"`
	NextAction        string               `json:"next_action"`
}

type ReviewProgress struct {
	Total             int                `json:"total"`
	Pending           int                `json:"pending"`
	Passed            int                `json:"passed"`
	Failed            int                `json:"failed"`
	LockedAtRevision  int                `json:"locked_at_revision,omitempty"`
	Overdue           int                `json:"overdue"`
	CompletionPercent float64            `json:"completion_percent"`
	ByReviewer        []ReviewerProgress `json:"by_reviewer"`
}

type ReviewerProgress struct {
	ReviewerID string `json:"reviewer_id"`
	Pending    int    `json:"pending"`
	Passed     int    `json:"passed"`
	Failed     int    `json:"failed"`
	Overdue    int    `json:"overdue"`
}

func ProjectSegments(batch *domain.ObservationBatch) []SegmentProjection {
	items := make([]SegmentProjection, 0, len(batch.Segments))
	for _, segment := range batch.Segments {
		item := SegmentProjection{SegmentID: segment.SegmentID, Status: segment.Status, SubmittedBy: segment.SubmittedBy, RegisteredAt: segment.RegisteredAt, ValidDurationSeconds: segment.ValidDurationSeconds, FileReference: segment.FileReference, ContentSHA256: segment.ContentSHA256, ReplacesSegmentID: segment.ReplacesSegmentID, IssueCodes: []string{}, ThresholdResults: []domain.ThresholdResult{}}
		if assessment := batch.Assessments[segment.SegmentID]; assessment != nil {
			item.QualityDecision = assessment.Decision
			item.IssueCodes = append(item.IssueCodes, assessment.IssueCodes...)
			item.ThresholdResults = append(item.ThresholdResults, assessment.ThresholdResults...)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].RegisteredAt.Equal(items[j].RegisteredAt) {
			return items[i].SegmentID < items[j].SegmentID
		}
		return items[i].RegisteredAt.Before(items[j].RegisteredAt)
	})
	return items
}

func ProjectIssues(batch *domain.ObservationBatch) []IssueProjection {
	items := make([]IssueProjection, 0, len(batch.Issues))
	for _, issue := range batch.Issues {
		segment := batch.Segments[issue.SegmentID]
		item := IssueProjection{IssueID: issue.IssueID, SegmentID: issue.SegmentID, Reason: issue.Reason, ReobservationPlan: issue.ReobservationPlan, ReplacementID: issue.ReplacementID, Open: issue.Open, IssueCodes: []string{}}
		if segment != nil {
			item.SegmentStatus = segment.Status
		}
		if assessment := batch.Assessments[issue.SegmentID]; assessment != nil {
			item.IssueCodes = append(item.IssueCodes, assessment.IssueCodes...)
		}
		switch {
		case !issue.Open:
			item.NextAction = "closed"
		case segment != nil && segment.Status == domain.SegmentFailed:
			item.NextAction = "quarantine"
		case segment != nil && segment.Status == domain.SegmentQuarantined:
			item.NextAction = "register_replacement"
		default:
			item.NextAction = "inspect"
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Open != items[j].Open {
			return items[i].Open
		}
		return items[i].SegmentID < items[j].SegmentID
	})
	return items
}

func ProjectReviewProgress(batch *domain.ObservationBatch) ReviewProgress {
	progress := ReviewProgress{Total: len(batch.ReviewItems), ByReviewer: []ReviewerProgress{}}
	stats := map[string]*ReviewerProgress{}
	now := time.Now()
	for _, item := range batch.ReviewItems {
		if item.LockedRevision > progress.LockedAtRevision {
			progress.LockedAtRevision = item.LockedRevision
		}
		switch item.Decision {
		case "passed":
			progress.Passed++
		case "failed":
			progress.Failed++
		default:
			progress.Pending++
		}
		key := item.ReviewerID
		if key == "" {
			key = "未分派"
		}
		entry := stats[key]
		if entry == nil {
			entry = &ReviewerProgress{ReviewerID: key}
			stats[key] = entry
		}
		switch item.Decision {
		case "passed":
			entry.Passed++
		case "failed":
			entry.Failed++
		default:
			entry.Pending++
			if item.DueAt != nil && item.DueAt.Before(now) {
				entry.Overdue++
				progress.Overdue++
			}
		}
	}
	if progress.Total > 0 {
		progress.CompletionPercent = float64(progress.Passed+progress.Failed) * 100 / float64(progress.Total)
	}
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		progress.ByReviewer = append(progress.ByReviewer, *stats[key])
	}
	return progress
}
