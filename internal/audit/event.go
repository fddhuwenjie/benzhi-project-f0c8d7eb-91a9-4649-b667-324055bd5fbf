package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Event struct {
	Sequence          uint64          `json:"sequence"`
	BatchID           string          `json:"batch_id"`
	AggregateRevision int             `json:"aggregate_revision"`
	Type              string          `json:"type"`
	Actor             string          `json:"actor"`
	OccurredAt        time.Time       `json:"occurred_at"`
	PreviousDigest    string          `json:"previous_digest"`
	Payload           json.RawMessage `json:"payload"`
	PayloadDigest     string          `json:"payload_digest"`
	EventDigest       string          `json:"event_digest"`
}

type eventBody struct {
	Sequence          uint64    `json:"sequence"`
	BatchID           string    `json:"batch_id"`
	AggregateRevision int       `json:"aggregate_revision"`
	Type              string    `json:"type"`
	Actor             string    `json:"actor"`
	OccurredAt        time.Time `json:"occurred_at"`
	PreviousDigest    string    `json:"previous_digest"`
	PayloadDigest     string    `json:"payload_digest"`
}

func NewEvent(sequence uint64, batchID string, revision int, typ, actor string, at time.Time, previous string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("编码审计载荷: %w", err)
	}
	payloadSum := sha256.Sum256(raw)
	e := Event{Sequence: sequence, BatchID: batchID, AggregateRevision: revision, Type: typ, Actor: actor, OccurredAt: at.UTC(), PreviousDigest: previous, Payload: raw, PayloadDigest: hex.EncodeToString(payloadSum[:])}
	digest, err := digestBody(e)
	if err != nil {
		return Event{}, err
	}
	e.EventDigest = digest
	return e, nil
}

func digestBody(e Event) (string, error) {
	body := eventBody{e.Sequence, e.BatchID, e.AggregateRevision, e.Type, e.Actor, e.OccurredAt, e.PreviousDigest, e.PayloadDigest}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func Verify(events []Event) error {
	previous := ""
	for i, e := range events {
		if e.Sequence != uint64(i+1) {
			return fmt.Errorf("事件序号不连续: 期望 %d，实际 %d", i+1, e.Sequence)
		}
		if e.PreviousDigest != previous {
			return fmt.Errorf("事件 %d 的前序摘要不匹配", e.Sequence)
		}
		payloadSum := sha256.Sum256(e.Payload)
		if hex.EncodeToString(payloadSum[:]) != e.PayloadDigest {
			return fmt.Errorf("事件 %d 的载荷摘要不匹配", e.Sequence)
		}
		digest, err := digestBody(e)
		if err != nil {
			return err
		}
		if digest != e.EventDigest {
			return fmt.Errorf("事件 %d 的事件摘要不匹配", e.Sequence)
		}
		previous = e.EventDigest
	}
	return nil
}
