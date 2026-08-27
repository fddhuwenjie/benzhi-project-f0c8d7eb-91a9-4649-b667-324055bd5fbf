package storage

import (
	"os"
	"testing"
	"time"

	"radio-observation-release-gate/internal/domain"
)

func TestRepositoryDetectsTruncatedEventFrame(t *testing.T) {
	repo, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := domain.NewBatch("B-1", "operator", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(batch, Commit{EventType: "batch.created", Actor: "operator", Payload: map[string]string{"request_id": "request-1", "batch_id": "B-1"}}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(repo.eventPath(batch.BatchID), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0, 10, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Load(batch.BatchID); err == nil {
		t.Fatal("truncated event log accepted")
	}
}

func TestManifestFileCannotBeOverwritten(t *testing.T) {
	repo, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &domain.ReleaseManifest{BatchID: "B-1", ManifestID: "M-1"}
	if err := repo.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveManifest(m); err != ErrManifestExists {
		t.Fatalf("expected ErrManifestExists, got %v", err)
	}
}

func TestStorageRejectsTraversalKey(t *testing.T) {
	repo, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Load("../escape"); err == nil {
		t.Fatal("path traversal key accepted")
	}
}
