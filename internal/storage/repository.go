package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"radio-observation-release-gate/internal/audit"
	"radio-observation-release-gate/internal/domain"
)

type Repository struct {
	root      string
	locks     sync.Map
	appenders sync.Map
	global    sync.Mutex
	clock     func() time.Time
}

func New(root string) (*Repository, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := os.MkdirAll(filepath.Join(root, "batches"), 0o750); err != nil {
		return nil, err
	}
	repo := &Repository{root: root, clock: time.Now}
	if err := repo.ensureIndex(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Root() string { return r.root }

func (r *Repository) Lock(batchID string) func() {
	value, _ := r.locks.LoadOrStore(batchID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (r *Repository) LockGlobal() func() {
	r.global.Lock()
	return r.global.Unlock
}

func (r *Repository) batchDir(id string) string { return filepath.Join(r.root, "batches", id) }
func (r *Repository) snapshotPath(id string) string {
	return filepath.Join(r.batchDir(id), "snapshot.json")
}
func (r *Repository) eventPath(id string) string { return filepath.Join(r.batchDir(id), "events.log") }
func (r *Repository) idempotencyPath(id string) string {
	return filepath.Join(r.batchDir(id), "idempotency.json")
}
func (r *Repository) manifestPath(id string) string {
	return filepath.Join(r.batchDir(id), "manifest.json")
}

func (r *Repository) Load(id string) (*domain.ObservationBatch, error) {
	if err := validateStorageID(id); err != nil {
		return nil, err
	}
	var batch domain.ObservationBatch
	if err := readJSON(r.snapshotPath(id), &batch); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if batch.Segments == nil {
		batch.Segments = map[string]*domain.ObservationSegment{}
	}
	if batch.Assessments == nil {
		batch.Assessments = map[string]*domain.QualityAssessment{}
	}
	if batch.Issues == nil {
		batch.Issues = map[string]*domain.QualityIssue{}
	}
	events, err := r.Events(id)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 || events[len(events)-1].AggregateRevision != batch.Revision {
		return nil, fmt.Errorf("批次 %s 的快照修订与事件链不一致", id)
	}
	if err := audit.ValidateSemantics(events); err != nil {
		return nil, fmt.Errorf("批次 %s 的事件语义不合法: %w", id, err)
	}
	if err := domain.ValidateAggregate(&batch); err != nil {
		return nil, fmt.Errorf("批次 %s 的聚合快照不合法: %w", id, err)
	}
	return &batch, nil
}

func (r *Repository) Exists(id string) (bool, error) {
	_, err := os.Stat(r.snapshotPath(id))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (r *Repository) Commit(batch *domain.ObservationBatch, c Commit) error {
	if err := validateStorageID(batch.BatchID); err != nil {
		return err
	}
	if err := domain.ValidateAggregate(batch); err != nil {
		return fmt.Errorf("拒绝持久化非法聚合: %w", err)
	}
	dir := r.batchDir(batch.BatchID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	events, err := r.Events(batch.BatchID)
	if err != nil {
		return err
	}
	var previous string
	if len(events) > 0 {
		previous = events[len(events)-1].EventDigest
	}
	event, err := audit.NewEvent(uint64(len(events)+1), batch.BatchID, batch.Revision, c.EventType, c.Actor, r.clock(), previous, c.Payload)
	if err != nil {
		return err
	}
	eventPath := r.eventPath(batch.BatchID)
	previousEventSize, err := fileSize(eventPath)
	if err != nil {
		return err
	}
	previousSnapshot, snapshotExisted, err := optionalFile(r.snapshotPath(batch.BatchID))
	if err != nil {
		return err
	}
	previousIdempotency, idempotencyExisted, err := optionalFile(r.idempotencyPath(batch.BatchID))
	if err != nil {
		return err
	}
	previousIndex, indexExisted, err := optionalFile(r.indexPath())
	if err != nil {
		return err
	}
	manifestCreated := false
	rollback := func() {
		_ = os.Truncate(eventPath, previousEventSize)
		_ = restoreFile(r.snapshotPath(batch.BatchID), previousSnapshot, snapshotExisted, 0o640)
		_ = restoreFile(r.idempotencyPath(batch.BatchID), previousIdempotency, idempotencyExisted, 0o640)
		_ = restoreFile(r.indexPath(), previousIndex, indexExisted, 0o640)
		if manifestCreated {
			_ = os.Remove(r.manifestPath(batch.BatchID))
		}
	}
	if c.Manifest != nil {
		if err := r.SaveManifest(c.Manifest); err != nil {
			return err
		}
		manifestCreated = true
	}
	if err := r.appendEvent(batch.BatchID, event); err != nil {
		rollback()
		return err
	}
	if err := atomicJSON(r.snapshotPath(batch.BatchID), batch, 0o640); err != nil {
		rollback()
		return err
	}
	if c.Idempotency != nil {
		records, err := r.loadIdempotency(batch.BatchID)
		if err != nil {
			rollback()
			return err
		}
		records[c.Idempotency.RequestID] = *c.Idempotency
		if err := atomicJSON(r.idempotencyPath(batch.BatchID), records, 0o640); err != nil {
			rollback()
			return err
		}
	}
	if c.EventType == "segment.registered" || c.EventType == "segments.registered" {
		if err := r.updateContentIndex(batch); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

func validateStorageID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return fmt.Errorf("批次编号不适合作为存储键")
	}
	return nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func optionalFile(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return raw, err == nil, err
}

func restoreFile(path string, raw []byte, existed bool, perm os.FileMode) error {
	if !existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, raw, perm)
}

func (r *Repository) appendEvent(id string, event audit.Event) error {
	value, ok := r.appenders.Load(id)
	if !ok {
		opened, err := os.OpenFile(r.eventPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		actual, loaded := r.appenders.LoadOrStore(id, opened)
		if loaded {
			_ = opened.Close()
		}
		value = actual
	}
	f := value.(*os.File)
	if err := audit.WriteFrame(f, event); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) Events(id string) ([]audit.Event, error) {
	f, err := os.Open(r.eventPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	events, err := audit.ReadFrames(f)
	if err != nil {
		return nil, err
	}
	if err := audit.Verify(events); err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.BatchID != id {
			return nil, fmt.Errorf("事件批次编号不匹配")
		}
	}
	return events, nil
}

func (r *Repository) loadIdempotency(id string) (map[string]IdempotencyRecord, error) {
	records := map[string]IdempotencyRecord{}
	err := readJSON(r.idempotencyPath(id), &records)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return records, nil
}

func (r *Repository) Idempotency(id, requestID string) (*IdempotencyRecord, error) {
	records, err := r.loadIdempotency(id)
	if err != nil {
		return nil, err
	}
	record, ok := records[requestID]
	if !ok {
		return nil, nil
	}
	return &record, nil
}

func (r *Repository) SaveManifest(m *domain.ReleaseManifest) error {
	if m == nil {
		return fmt.Errorf("发布清单不能为空")
	}
	if err := validateStorageID(m.BatchID); err != nil {
		return err
	}
	if err := os.MkdirAll(r.batchDir(m.BatchID), 0o750); err != nil {
		return err
	}
	path := r.manifestPath(m.BatchID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o440)
	if err != nil {
		if os.IsExist(err) {
			return ErrManifestExists
		}
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return syncDir(filepath.Dir(path))
}

func (r *Repository) Manifest(id string) (*domain.ReleaseManifest, error) {
	var m domain.ReleaseManifest
	if err := readJSON(r.manifestPath(id), &m); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (r *Repository) List() ([]*domain.ObservationBatch, error) {
	entries, err := os.ReadDir(filepath.Join(r.root, "batches"))
	if err != nil {
		return nil, err
	}
	result := make([]*domain.ObservationBatch, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		batch, err := r.Load(entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, batch)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (r *Repository) FindContentDigest(digest, exceptBatch string) (string, bool, error) {
	location, err := r.ContentLocation(digest)
	if err != nil {
		return "", false, err
	}
	if location != nil && location.BatchID != exceptBatch {
		return location.BatchID, true, nil
	}
	return "", false, nil
}
