package application

import (
	"radio-observation-release-gate/internal/audit"
	"radio-observation-release-gate/internal/domain"
	"time"
)

type Action struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

type BatchView struct {
	Batch                    *domain.ObservationBatch `json:"batch"`
	EffectiveCoverageSeconds float64                  `json:"effective_coverage_seconds"`
	Coverage                 domain.CoverageReport    `json:"coverage"`
	Eligibility              domain.EligibilityReport `json:"eligibility"`
	Segments                 []SegmentProjection      `json:"segments"`
	IssueQueue               []IssueProjection        `json:"issue_queue"`
	ReviewProgress           ReviewProgress           `json:"review_progress"`
	OpenIssueCount           int                      `json:"open_issue_count"`
	PendingReviewCount       int                      `json:"pending_review_count"`
	Actions                  []Action                 `json:"actions"`
}

type CommandResult struct {
	Batch          *domain.ObservationBatch    `json:"batch"`
	Replayed       bool                        `json:"replayed"`
	Registered     []RegisteredSegmentResult   `json:"registered,omitempty"`
	Assessments    []*domain.QualityAssessment `json:"assessments,omitempty"`
	QualitySummary *domain.BatchQualitySummary `json:"quality_summary,omitempty"`
	auditPayload   any
}

type RegisteredSegmentResult struct {
	InputIndex int                  `json:"input_index"`
	SegmentID  string               `json:"segment_id"`
	Status     domain.SegmentStatus `json:"status"`
}

type BatchListFilter struct {
	BatchID      string
	TelescopeID  string
	TargetSource string
	State        domain.BatchState
	TodoOnly     bool
	Page         int
	PageSize     int
}
type BatchListItem struct {
	BatchView
	CoverageGapSeconds float64   `json:"coverage_gap_seconds"`
	TerminalReadOnly   bool      `json:"terminal_read_only"`
	UpdatedAt          time.Time `json:"updated_at"`
}
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}
type BatchListResult struct {
	Batches     []BatchListItem           `json:"batches"`
	StateCounts map[domain.BatchState]int `json:"state_counts"`
	TodoTotal   int                       `json:"todo_total"`
	Pagination  Pagination                `json:"pagination"`
	Filter      BatchListFilterView       `json:"filter"`
}
type BatchListFilterView struct {
	BatchID      string            `json:"batch_id,omitempty"`
	TelescopeID  string            `json:"telescope_id,omitempty"`
	TargetSource string            `json:"target_source,omitempty"`
	State        domain.BatchState `json:"state,omitempty"`
	TodoOnly     bool              `json:"todo_only"`
}

type ManifestVerification struct {
	BatchID          string `json:"batch_id"`
	StoredSHA256     string `json:"stored_sha256"`
	RecomputedSHA256 string `json:"recomputed_sha256"`
	Valid            bool   `json:"valid"`
}

type TimelineView struct {
	Items     []audit.TimelineItem  `json:"items"`
	Integrity audit.IntegrityReport `json:"integrity"`
}

func BuildView(batch *domain.ObservationBatch) BatchView {
	open, pending := 0, 0
	for _, issue := range batch.Issues {
		if issue.Open {
			open++
		}
	}
	for _, item := range batch.ReviewItems {
		if item.Decision == "" {
			pending++
		}
	}
	terminal := batch.Terminal()
	eligibility := domain.EvaluateEligibility(batch)
	actions := []Action{
		{Code: "freeze", Label: "冻结基线", Enabled: batch.State == domain.StateDraft},
		{Code: "register_segment", Label: "登记数据段", Enabled: eligibility.CanRegisterSegments},
		{Code: "generate_review", Label: "生成抽审清单", Enabled: eligibility.CanGenerateReview},
		{Code: "seal", Label: "封存终态", Enabled: eligibility.CanSeal},
	}
	if terminal {
		for i := range actions {
			actions[i].Enabled = false
			actions[i].Reason = "终态只读"
		}
	}
	return BatchView{Batch: batch, EffectiveCoverageSeconds: eligibility.Coverage.EffectiveSeconds, Coverage: eligibility.Coverage, Eligibility: eligibility, Segments: ProjectSegments(batch), IssueQueue: ProjectIssues(batch), ReviewProgress: ProjectReviewProgress(batch), OpenIssueCount: open, PendingReviewCount: pending, Actions: actions}
}
