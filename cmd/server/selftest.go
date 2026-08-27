package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
	"radio-observation-release-gate/internal/web"
)

type selftestClient struct {
	base     string
	client   *http.Client
	revision int
}

func runSelftest(address string) error {
	dir, err := os.MkdirTemp("", "radio-release-gate-selftest-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	repo, err := storage.New(dir)
	if err != nil {
		return err
	}
	app := application.New(repo)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听自检地址 %s: %w", address, err)
	}
	server := &http.Server{Handler: web.New(app, slog.New(slog.NewTextHandler(io.Discard, nil))), ReadHeaderTimeout: 2 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		<-done
	}()
	c := &selftestClient{base: "http://" + listener.Addr().String(), client: &http.Client{Timeout: 4 * time.Second}}
	if err := c.getRoot(); err != nil {
		return err
	}
	if err := c.post("/api/batches", map[string]any{"request_id": "st-create", "expected_revision": 0, "actor": "operator-a", "batch_id": "SELFTEST-001"}); err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-30 * time.Minute)
	end := now.Add(30 * time.Minute)
	baseline := domain.Baseline{TelescopeID: "RT-SELFTEST", TargetSource: "PSR-B0329+54", FrequencyBand: "L-band", FrequencyLowHz: 1.2e9, FrequencyHighHz: 1.6e9, PlannedWindow: domain.PlannedWindow{Start: start, End: end}, MinimumValidDuration: 600, QualityThresholds: domain.QualityThresholds{MaxRFIOccupancy: .2, MinCompleteness: .95, MaxPacketLoss: .05, MinSignalToNoise: 8}, SamplingSeed: "selftest-seed", SampleSize: 1}
	if err := c.post("/api/batches/SELFTEST-001/freeze", map[string]any{"request_id": "st-freeze", "expected_revision": c.revision, "actor": "operator-a", "baseline": baseline}); err != nil {
		return err
	}
	content := sha256.Sum256([]byte("selftest-observation-segment"))
	segment := domain.ObservationSegment{SegmentID: "SEG-001", StartTime: start, EndTime: end, FrequencyLowHz: 1.25e9, FrequencyHighHz: 1.55e9, ValidDurationSeconds: 1800, FileReference: "local://selftest/segment-001.fits", ContentSHA256: hex.EncodeToString(content[:]), SubmittedBy: "operator-a", Metrics: domain.SegmentMetrics{RFIOccupancy: .08, Completeness: .99, PacketLoss: .01, SignalToNoise: 14}}
	if err := c.post("/api/batches/SELFTEST-001/segments", map[string]any{"request_id": "st-segment", "expected_revision": c.revision, "actor": "operator-a", "segment": segment}); err != nil {
		return err
	}
	if err := c.post("/api/batches/SELFTEST-001/quality", map[string]any{"request_id": "st-quality", "expected_revision": c.revision, "actor": "operator-a", "segment_id": "SEG-001"}); err != nil {
		return err
	}
	if err := c.post("/api/batches/SELFTEST-001/reviews", map[string]any{"request_id": "st-sample", "expected_revision": c.revision, "actor": "operator-a"}); err != nil {
		return err
	}
	if err := c.post("/api/batches/SELFTEST-001/review-assignments", map[string]any{"request_id": "st-assign", "expected_revision": c.revision, "actor": "supervisor", "review_item_id": "review-SEG-001", "reviewer_id": "reviewer-b", "due_at": time.Now().UTC().Add(time.Hour), "reason": "自检独立复核分派"}); err != nil {
		return err
	}
	if err := c.post("/api/batches/SELFTEST-001/review-decisions", map[string]any{"request_id": "st-review", "expected_revision": c.revision, "actor": "reviewer-b", "review_item_id": "review-SEG-001", "evidence_note": "文件摘要、频段与质检证据一致", "decision": "passed"}); err != nil {
		return err
	}
	if err := c.post("/api/batches/SELFTEST-001/seal", map[string]any{"request_id": "st-seal", "expected_revision": c.revision, "actor": "reviewer-b"}); err != nil {
		return err
	}
	var verification application.ManifestVerification
	if err := c.get("/api/batches/SELFTEST-001/manifest/verify", &verification); err != nil {
		return err
	}
	if !verification.Valid {
		return fmt.Errorf("发布清单摘要验证失败")
	}
	var view application.BatchView
	if err := c.get("/api/batches/SELFTEST-001", &view); err != nil {
		return err
	}
	if view.Batch.State != domain.StateApproved {
		return fmt.Errorf("自检终态不正确: %s", view.Batch.State)
	}
	return nil
}

func (c *selftestClient) getRoot() error {
	response, err := c.client.Get(c.base + "/")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("工作台入口返回 %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if !bytes.Contains(raw, []byte("观测发布资格工作台")) {
		return fmt.Errorf("工作台 HTML 内容不完整")
	}
	return nil
}

func (c *selftestClient) post(path string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("POST %s 返回 %d: %s", path, response.StatusCode, string(body))
	}
	var result application.CommandResult
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if result.Batch == nil {
		return fmt.Errorf("POST %s 未返回批次", path)
	}
	c.revision = result.Batch.Revision
	return nil
}

func (c *selftestClient) get(path string, dst any) error {
	response, err := c.client.Get(c.base + path)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("GET %s 返回 %d: %s", path, response.StatusCode, string(body))
	}
	return json.Unmarshal(body, dst)
}
