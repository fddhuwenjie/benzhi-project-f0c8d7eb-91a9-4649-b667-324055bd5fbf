package audit

import (
	"fmt"
	"sort"
	"time"
)

type RevisionSpan struct {
	Revision      int       `json:"revision"`
	FirstSequence uint64    `json:"first_sequence"`
	LastSequence  uint64    `json:"last_sequence"`
	FirstEventAt  time.Time `json:"first_event_at"`
	LastEventAt   time.Time `json:"last_event_at"`
}

type IntegrityReport struct {
	BatchID          string         `json:"batch_id"`
	EventCount       int            `json:"event_count"`
	FirstSequence    uint64         `json:"first_sequence"`
	LastSequence     uint64         `json:"last_sequence"`
	HeadDigest       string         `json:"head_digest"`
	RevisionSpans    []RevisionSpan `json:"revision_spans"`
	CommandTypeCount map[string]int `json:"command_type_count"`
	Valid            bool           `json:"valid"`
}

func Inspect(events []Event) (IntegrityReport, error) {
	report := IntegrityReport{RevisionSpans: []RevisionSpan{}, CommandTypeCount: map[string]int{}}
	if len(events) == 0 {
		return report, nil
	}
	if err := Verify(events); err != nil {
		return report, err
	}
	if err := ValidateSemantics(events); err != nil {
		return report, err
	}
	report.BatchID = events[0].BatchID
	report.EventCount = len(events)
	report.FirstSequence = events[0].Sequence
	report.LastSequence = events[len(events)-1].Sequence
	report.HeadDigest = events[len(events)-1].EventDigest
	spans := map[int]*RevisionSpan{}
	lastRevision := 0
	for _, event := range events {
		if event.BatchID != report.BatchID {
			return report, fmt.Errorf("事件链混入其他批次 %s", event.BatchID)
		}
		if event.AggregateRevision <= lastRevision {
			return report, fmt.Errorf("聚合修订未严格递增: %d", event.AggregateRevision)
		}
		lastRevision = event.AggregateRevision
		report.CommandTypeCount[event.Type]++
		span := spans[event.AggregateRevision]
		if span == nil {
			span = &RevisionSpan{Revision: event.AggregateRevision, FirstSequence: event.Sequence, FirstEventAt: event.OccurredAt}
			spans[event.AggregateRevision] = span
		}
		span.LastSequence = event.Sequence
		span.LastEventAt = event.OccurredAt
	}
	revisions := make([]int, 0, len(spans))
	for revision := range spans {
		revisions = append(revisions, revision)
	}
	sort.Ints(revisions)
	for _, revision := range revisions {
		report.RevisionSpans = append(report.RevisionSpans, *spans[revision])
	}
	report.Valid = true
	return report, nil
}

func HeadDigest(events []Event) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].EventDigest
}
