package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"radio-observation-release-gate/internal/domain"
)

type ContentLocation struct {
	BatchID       string    `json:"batch_id"`
	SegmentID     string    `json:"segment_id"`
	FileReference string    `json:"file_reference"`
	RegisteredAt  time.Time `json:"registered_at"`
}

type ContentIndex struct {
	Version int                        `json:"version"`
	Entries map[string]ContentLocation `json:"entries"`
}

func (r *Repository) indexPath() string { return filepath.Join(r.root, "content-index.json") }

func (r *Repository) ensureIndex() error {
	var index ContentIndex
	if err := readJSON(r.indexPath(), &index); err == nil {
		return validateContentIndex(index)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取内容摘要索引: %w", err)
	}
	return r.RebuildContentIndex()
}

func (r *Repository) RebuildContentIndex() error {
	entries, err := os.ReadDir(filepath.Join(r.root, "batches"))
	if err != nil {
		return err
	}
	index := ContentIndex{Version: 1, Entries: map[string]ContentLocation{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var batch domain.ObservationBatch
		if err := readJSON(r.snapshotPath(entry.Name()), &batch); err != nil {
			return err
		}
		ids := make([]string, 0, len(batch.Segments))
		for id := range batch.Segments {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			segment := batch.Segments[id]
			digest := strings.ToLower(segment.ContentSHA256)
			if existing, ok := index.Entries[digest]; ok && existing.BatchID != batch.BatchID {
				return fmt.Errorf("摘要 %s 同时属于批次 %s 和 %s", digest, existing.BatchID, batch.BatchID)
			}
			index.Entries[digest] = ContentLocation{BatchID: batch.BatchID, SegmentID: segment.SegmentID, FileReference: segment.FileReference, RegisteredAt: segment.RegisteredAt}
		}
	}
	return atomicJSON(r.indexPath(), index, 0o640)
}

func (r *Repository) ContentLocation(digest string) (*ContentLocation, error) {
	var index ContentIndex
	if err := readJSON(r.indexPath(), &index); err != nil {
		return nil, err
	}
	if err := validateContentIndex(index); err != nil {
		return nil, err
	}
	location, exists := index.Entries[strings.ToLower(digest)]
	if !exists {
		return nil, nil
	}
	return &location, nil
}

func (r *Repository) updateContentIndex(batch *domain.ObservationBatch) error {
	var index ContentIndex
	if err := readJSON(r.indexPath(), &index); err != nil {
		return err
	}
	for _, segment := range batch.Segments {
		digest := strings.ToLower(segment.ContentSHA256)
		if existing, exists := index.Entries[digest]; exists && existing.BatchID != batch.BatchID {
			return fmt.Errorf("内容摘要已经登记到批次 %s", existing.BatchID)
		}
		index.Entries[digest] = ContentLocation{BatchID: batch.BatchID, SegmentID: segment.SegmentID, FileReference: segment.FileReference, RegisteredAt: segment.RegisteredAt}
	}
	return atomicJSON(r.indexPath(), index, 0o640)
}

func validateContentIndex(index ContentIndex) error {
	if index.Version != 1 || index.Entries == nil {
		return fmt.Errorf("内容摘要索引版本或结构不合法")
	}
	for digest, location := range index.Entries {
		if len(digest) != 64 || strings.TrimSpace(location.BatchID) == "" || strings.TrimSpace(location.SegmentID) == "" {
			return fmt.Errorf("内容摘要索引条目损坏")
		}
	}
	return nil
}
