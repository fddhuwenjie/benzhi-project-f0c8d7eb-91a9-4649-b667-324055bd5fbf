package audit

import (
	"bytes"
	"testing"
	"time"
)

func TestFrameAndDigestChain(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	first, err := NewEvent(1, "B-1", 1, "batch.created", "operator", now, "", map[string]string{"request_id": "request-1", "value": "one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEvent(2, "B-1", 2, "baseline.frozen", "operator", now.Add(time.Second), first.EventDigest, map[string]string{"request_id": "request-2", "value": "two"})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := WriteFrame(&buffer, first); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(&buffer, second); err != nil {
		t.Fatal(err)
	}
	events, err := ReadFrames(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(events); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(events)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.EventCount != 2 || report.HeadDigest != second.EventDigest {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestReadFramesRejectsTruncation(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	event, _ := NewEvent(1, "B-1", 1, "batch.created", "operator", now, "", map[string]string{"request_id": "request-1", "value": "one"})
	var buffer bytes.Buffer
	_ = WriteFrame(&buffer, event)
	raw := buffer.Bytes()
	if _, err := ReadFrames(bytes.NewReader(raw[:len(raw)-2])); err == nil {
		t.Fatal("truncated event frame accepted")
	}
}
