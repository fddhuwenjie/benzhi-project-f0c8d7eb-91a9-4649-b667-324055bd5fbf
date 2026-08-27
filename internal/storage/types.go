package storage

import (
	"encoding/json"
	"errors"

	"radio-observation-release-gate/internal/domain"
)

var (
	ErrNotFound       = errors.New("批次不存在")
	ErrManifestExists = errors.New("发布清单已经封存")
)

type IdempotencyRecord struct {
	RequestID   string          `json:"request_id"`
	Fingerprint string          `json:"fingerprint"`
	Status      int             `json:"status"`
	Response    json.RawMessage `json:"response"`
}

type Commit struct {
	EventType   string
	Actor       string
	Payload     any
	Idempotency *IdempotencyRecord
	Manifest    *domain.ReleaseManifest
}
