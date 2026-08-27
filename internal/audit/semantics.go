package audit

import (
	"encoding/json"
	"fmt"
	"strings"
)

type EventDefinition struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Terminal    bool   `json:"terminal"`
}

var eventDefinitions = map[string]EventDefinition{
	"batch.created":          {Type: "batch.created", DisplayName: "批次建档"},
	"baseline.frozen":        {Type: "baseline.frozen", DisplayName: "基线冻结"},
	"segment.registered":     {Type: "segment.registered", DisplayName: "数据段登记"},
	"segments.registered":    {Type: "segments.registered", DisplayName: "数据段批量登记"},
	"quality.assessed":       {Type: "quality.assessed", DisplayName: "确定性质检"},
	"quality.batch_assessed": {Type: "quality.batch_assessed", DisplayName: "整批确定性质检"},
	"segment.quarantined":    {Type: "segment.quarantined", DisplayName: "数据段隔离"},
	"review.generated":       {Type: "review.generated", DisplayName: "抽审清单锁定"},
	"review.decided":         {Type: "review.decided", DisplayName: "抽审项目裁定"},
	"review.assigned":        {Type: "review.assigned", DisplayName: "抽审任务分派"},
	"batch.sealed":           {Type: "batch.sealed", DisplayName: "终态清单封存", Terminal: true},
}

func Definition(eventType string) (EventDefinition, bool) {
	definition, ok := eventDefinitions[eventType]
	return definition, ok
}

func ValidateSemantics(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	if events[0].Type != "batch.created" || events[0].AggregateRevision != 1 {
		return fmt.Errorf("事件链必须从修订 1 的 batch.created 开始")
	}
	terminalSeen := false
	for index, event := range events {
		definition, known := Definition(event.Type)
		if !known {
			return fmt.Errorf("事件 %d 使用未知命令类型 %q", event.Sequence, event.Type)
		}
		if terminalSeen {
			return fmt.Errorf("终态事件后仍存在写命令")
		}
		if strings.TrimSpace(event.Actor) == "" {
			return fmt.Errorf("事件 %d 缺少操作人", event.Sequence)
		}
		if event.AggregateRevision != index+1 {
			return fmt.Errorf("事件 %d 的聚合修订不连续", event.Sequence)
		}
		if index > 0 && event.OccurredAt.Before(events[index-1].OccurredAt) {
			return fmt.Errorf("事件 %d 的发生时间早于前一事件", event.Sequence)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("事件 %d 的命令载荷不是对象", event.Sequence)
		}
		var requestID string
		if raw := payload["request_id"]; raw != nil {
			_ = json.Unmarshal(raw, &requestID)
		}
		if strings.TrimSpace(requestID) == "" {
			return fmt.Errorf("事件 %d 缺少 request_id", event.Sequence)
		}
		terminalSeen = definition.Terminal
	}
	return nil
}

func EventDisplayName(eventType string) string {
	if definition, ok := Definition(eventType); ok {
		return definition.DisplayName
	}
	return eventType
}
