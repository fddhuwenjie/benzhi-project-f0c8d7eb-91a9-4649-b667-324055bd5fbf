package domain

import "time"

type BatchState string

const (
	StateDraft       BatchState = "draft"
	StateFrozen      BatchState = "frozen"
	StateQuality     BatchState = "quality_checked"
	StateRemediation BatchState = "remediation"
	StateReview      BatchState = "under_review"
	StateApproved    BatchState = "approved"
	StateRejected    BatchState = "rejected"
)

type SegmentStatus string

const (
	SegmentRegistered  SegmentStatus = "registered"
	SegmentPassed      SegmentStatus = "passed"
	SegmentFailed      SegmentStatus = "failed"
	SegmentQuarantined SegmentStatus = "quarantined"
	SegmentReplaced    SegmentStatus = "replaced"
)

type PlannedWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type QualityThresholds struct {
	MaxRFIOccupancy  float64 `json:"max_rfi_occupancy"`
	MinCompleteness  float64 `json:"min_completeness"`
	MaxPacketLoss    float64 `json:"max_packet_loss"`
	MinSignalToNoise float64 `json:"min_signal_to_noise"`
}

type Baseline struct {
	TelescopeID          string            `json:"telescope_id"`
	TargetSource         string            `json:"target_source"`
	FrequencyBand        string            `json:"frequency_band"`
	FrequencyLowHz       float64           `json:"frequency_low_hz"`
	FrequencyHighHz      float64           `json:"frequency_high_hz"`
	PlannedWindow        PlannedWindow     `json:"planned_window"`
	MinimumValidDuration float64           `json:"minimum_valid_duration_seconds"`
	QualityThresholds    QualityThresholds `json:"quality_thresholds"`
	SamplingSeed         string            `json:"sampling_seed"`
	SampleSize           int               `json:"sample_size"`
	Version              int               `json:"version"`
	FrozenAt             *time.Time        `json:"frozen_at,omitempty"`
}

type SegmentMetrics struct {
	RFIOccupancy  float64 `json:"rfi_occupancy"`
	Completeness  float64 `json:"completeness"`
	PacketLoss    float64 `json:"packet_loss"`
	SignalToNoise float64 `json:"signal_to_noise"`
}

type ObservationSegment struct {
	SegmentID            string         `json:"segment_id"`
	BatchID              string         `json:"batch_id"`
	StartTime            time.Time      `json:"start_time"`
	EndTime              time.Time      `json:"end_time"`
	FrequencyLowHz       float64        `json:"frequency_low_hz"`
	FrequencyHighHz      float64        `json:"frequency_high_hz"`
	ValidDurationSeconds float64        `json:"valid_duration_seconds"`
	FileReference        string         `json:"file_reference"`
	ContentSHA256        string         `json:"content_sha256"`
	ReplacesSegmentID    string         `json:"replaces_segment_id,omitempty"`
	Status               SegmentStatus  `json:"status"`
	SubmittedBy          string         `json:"submitted_by"`
	Metrics              SegmentMetrics `json:"metrics"`
	RegisteredAt         time.Time      `json:"registered_at"`
}

type ThresholdResult struct {
	Metric    string  `json:"metric"`
	Measured  float64 `json:"measured"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	Passed    bool    `json:"passed"`
}

type QualityAssessment struct {
	AssessmentID     string            `json:"assessment_id"`
	BatchID          string            `json:"batch_id"`
	SegmentID        string            `json:"segment_id"`
	BaselineRevision int               `json:"baseline_revision"`
	Metrics          SegmentMetrics    `json:"metrics"`
	IssueCodes       []string          `json:"issue_codes"`
	ThresholdResults []ThresholdResult `json:"threshold_results"`
	Decision         string            `json:"decision"`
	EvaluatedAt      time.Time         `json:"evaluated_at"`
}

type QualityIssue struct {
	IssueID           string     `json:"issue_id"`
	SegmentID         string     `json:"segment_id"`
	Reason            string     `json:"reason,omitempty"`
	ReobservationPlan string     `json:"reobservation_plan,omitempty"`
	ReplacementID     string     `json:"replacement_id,omitempty"`
	Open              bool       `json:"open"`
	CreatedAt         time.Time  `json:"created_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

type ReviewItem struct {
	ReviewItemID      string             `json:"review_item_id"`
	BatchID           string             `json:"batch_id"`
	SegmentID         string             `json:"segment_id"`
	SampleRank        int                `json:"sample_rank"`
	LockedRevision    int                `json:"locked_revision"`
	ReviewerID        string             `json:"reviewer_id,omitempty"`
	EvidenceNote      string             `json:"evidence_note,omitempty"`
	Decision          string             `json:"decision,omitempty"`
	ReviewedAt        *time.Time         `json:"reviewed_at,omitempty"`
	DueAt             *time.Time         `json:"due_at,omitempty"`
	AssignedBy        string             `json:"assigned_by,omitempty"`
	AssignmentReason  string             `json:"assignment_reason,omitempty"`
	AssignmentHistory []ReviewAssignment `json:"assignment_history,omitempty"`
}

type ReviewAssignment struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	From   string    `json:"from,omitempty"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
	DueAt  time.Time `json:"due_at"`
}

type ManifestSegment struct {
	SegmentID     string `json:"segment_id"`
	FileReference string `json:"file_reference"`
	ContentSHA256 string `json:"content_sha256"`
}

type ReleaseManifest struct {
	ManifestID       string            `json:"manifest_id"`
	BatchID          string            `json:"batch_id"`
	TerminalDecision string            `json:"terminal_decision"`
	SegmentDigests   []ManifestSegment `json:"segment_digests"`
	BaselineDigest   string            `json:"baseline_digest"`
	ReviewDigest     string            `json:"review_digest"`
	CanonicalSHA256  string            `json:"canonical_sha256"`
	SealedBy         string            `json:"sealed_by"`
	SealedAt         time.Time         `json:"sealed_at"`
}

type ObservationBatch struct {
	BatchID     string                         `json:"batch_id"`
	Baseline    Baseline                       `json:"baseline"`
	State       BatchState                     `json:"state"`
	Revision    int                            `json:"revision"`
	CreatedBy   string                         `json:"created_by"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
	Segments    map[string]*ObservationSegment `json:"segments"`
	Assessments map[string]*QualityAssessment  `json:"assessments"`
	Issues      map[string]*QualityIssue       `json:"issues"`
	ReviewItems []*ReviewItem                  `json:"review_items"`
	Manifest    *ReleaseManifest               `json:"manifest,omitempty"`
}

func (b *ObservationBatch) Terminal() bool {
	return b.State == StateApproved || b.State == StateRejected
}
