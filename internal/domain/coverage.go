package domain

import (
	"sort"
	"time"
)

type CoverageInterval struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationSeconds float64   `json:"duration_seconds"`
	SegmentIDs      []string  `json:"segment_ids"`
}

type CoverageGap struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationSeconds float64   `json:"duration_seconds"`
}

type CoverageReport struct {
	EffectiveSeconds float64            `json:"effective_seconds"`
	RequiredSeconds  float64            `json:"required_seconds"`
	Sufficient       bool               `json:"sufficient"`
	Intervals        []CoverageInterval `json:"intervals"`
	Gaps             []CoverageGap      `json:"gaps"`
}

type coveragePoint struct {
	start time.Time
	end   time.Time
	id    string
}

func BuildCoverageReport(b *ObservationBatch) CoverageReport {
	points := make([]coveragePoint, 0)
	for _, segment := range b.Segments {
		if segment.Status == SegmentPassed {
			points = append(points, coveragePoint{start: segment.StartTime, end: segment.EndTime, id: segment.SegmentID})
		}
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].start.Equal(points[j].start) {
			return points[i].id < points[j].id
		}
		return points[i].start.Before(points[j].start)
	})
	report := CoverageReport{RequiredSeconds: b.Baseline.MinimumValidDuration, Intervals: []CoverageInterval{}, Gaps: []CoverageGap{}}
	if len(points) == 0 {
		if !b.Baseline.PlannedWindow.Start.IsZero() {
			report.Gaps = append(report.Gaps, CoverageGap{Start: b.Baseline.PlannedWindow.Start, End: b.Baseline.PlannedWindow.End, DurationSeconds: b.Baseline.PlannedWindow.End.Sub(b.Baseline.PlannedWindow.Start).Seconds()})
		}
		return report
	}
	current := CoverageInterval{Start: points[0].start, End: points[0].end, SegmentIDs: []string{points[0].id}}
	for _, point := range points[1:] {
		if !point.start.After(current.End) {
			if point.end.After(current.End) {
				current.End = point.end
			}
			current.SegmentIDs = append(current.SegmentIDs, point.id)
			continue
		}
		current.DurationSeconds = current.End.Sub(current.Start).Seconds()
		report.Intervals = append(report.Intervals, current)
		current = CoverageInterval{Start: point.start, End: point.end, SegmentIDs: []string{point.id}}
	}
	current.DurationSeconds = current.End.Sub(current.Start).Seconds()
	report.Intervals = append(report.Intervals, current)
	for _, interval := range report.Intervals {
		report.EffectiveSeconds += interval.DurationSeconds
	}
	cursor := b.Baseline.PlannedWindow.Start
	for _, interval := range report.Intervals {
		if interval.Start.After(cursor) {
			report.Gaps = append(report.Gaps, CoverageGap{Start: cursor, End: interval.Start, DurationSeconds: interval.Start.Sub(cursor).Seconds()})
		}
		if interval.End.After(cursor) {
			cursor = interval.End
		}
	}
	if cursor.Before(b.Baseline.PlannedWindow.End) {
		report.Gaps = append(report.Gaps, CoverageGap{Start: cursor, End: b.Baseline.PlannedWindow.End, DurationSeconds: b.Baseline.PlannedWindow.End.Sub(cursor).Seconds()})
	}
	report.Sufficient = report.EffectiveSeconds >= report.RequiredSeconds
	return report
}
