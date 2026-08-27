package audit

import (
	"encoding/json"
	"sort"
	"time"
)

type TimelineItem struct {
	Sequence       uint64          `json:"sequence"`
	Revision       int             `json:"revision"`
	Type           string          `json:"type"`
	Actor          string          `json:"actor"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Digest         string          `json:"digest"`
	Summary        json.RawMessage `json:"summary"`
	PreviousDigest string          `json:"previous_digest"`
	DisplayName    string          `json:"display_name"`
}

func ProjectTimeline(events []Event) []TimelineItem {
	items := make([]TimelineItem, 0, len(events))
	for _, e := range events {
		items = append(items, TimelineItem{Sequence: e.Sequence, Revision: e.AggregateRevision, Type: e.Type, Actor: e.Actor, OccurredAt: e.OccurredAt, Digest: e.EventDigest, Summary: e.Payload, PreviousDigest: e.PreviousDigest, DisplayName: EventDisplayName(e.Type)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	return items
}
