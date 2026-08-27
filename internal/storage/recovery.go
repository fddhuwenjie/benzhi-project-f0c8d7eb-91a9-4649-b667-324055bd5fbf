package storage

import "fmt"

type RecoveryResult struct {
	Batches int `json:"batches"`
	Events  int `json:"events"`
}

func (r *Repository) VerifyAll() (RecoveryResult, error) {
	batches, err := r.List()
	if err != nil {
		return RecoveryResult{}, err
	}
	result := RecoveryResult{Batches: len(batches)}
	for _, batch := range batches {
		events, err := r.Events(batch.BatchID)
		if err != nil {
			return result, fmt.Errorf("校验批次 %s: %w", batch.BatchID, err)
		}
		result.Events += len(events)
		if batch.Manifest != nil {
			stored, err := r.Manifest(batch.BatchID)
			if err != nil {
				return result, err
			}
			if stored.CanonicalSHA256 != batch.Manifest.CanonicalSHA256 {
				return result, fmt.Errorf("批次 %s 的清单摘要与快照不一致", batch.BatchID)
			}
		}
	}
	var index ContentIndex
	if err := readJSON(r.indexPath(), &index); err != nil {
		return result, err
	}
	if err := validateContentIndex(index); err != nil {
		return result, err
	}
	for digest, location := range index.Entries {
		batch, err := r.Load(location.BatchID)
		if err != nil {
			return result, fmt.Errorf("摘要索引批次不存在: %w", err)
		}
		segment := batch.Segments[location.SegmentID]
		if segment == nil || segment.ContentSHA256 != digest {
			return result, fmt.Errorf("摘要索引与数据段不一致: %s", digest)
		}
	}
	return result, nil
}
