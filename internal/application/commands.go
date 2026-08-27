package application

import (
	"radio-observation-release-gate/internal/domain"
	"time"
)

type RequestMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int    `json:"expected_revision"`
	Actor            string `json:"actor"`
}

type CreateBatchCommand struct {
	RequestMeta
	BatchID string `json:"batch_id"`
}

type FreezeBaselineCommand struct {
	RequestMeta
	Baseline domain.Baseline `json:"baseline"`
}

type RegisterSegmentCommand struct {
	RequestMeta
	Segment domain.ObservationSegment `json:"segment"`
}

type RegisterSegmentsCommand struct {
	RequestMeta
	Segments []domain.ObservationSegment `json:"segments"`
}

type AssessSegmentCommand struct {
	RequestMeta
	SegmentID string `json:"segment_id"`
}

type AssessBatchCommand struct{ RequestMeta }

type PreviewReplacementCommand struct {
	ExpectedRevision int                       `json:"expected_revision"`
	IssueID          string                    `json:"issue_id"`
	Segment          domain.ObservationSegment `json:"segment"`
}

type QuarantineCommand struct {
	RequestMeta
	SegmentID         string `json:"segment_id"`
	Reason            string `json:"reason"`
	ReobservationPlan string `json:"reobservation_plan"`
}

type GenerateReviewCommand struct{ RequestMeta }

type DecideReviewCommand struct {
	RequestMeta
	ReviewItemID string `json:"review_item_id"`
	EvidenceNote string `json:"evidence_note"`
	Decision     string `json:"decision"`
}

type AssignReviewCommand struct {
	RequestMeta
	ReviewItemID string    `json:"review_item_id"`
	ReviewerID   string    `json:"reviewer_id"`
	DueAt        time.Time `json:"due_at"`
	Reason       string    `json:"reason"`
}

type SealCommand struct{ RequestMeta }
