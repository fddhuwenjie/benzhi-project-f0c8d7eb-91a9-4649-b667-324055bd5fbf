package audit

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxEventFrame = 4 << 20

func WriteFrame(w io.Writer, e Event) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(raw) > MaxEventFrame {
		return fmt.Errorf("审计事件超过大小限制")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(raw)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(raw)
	return err
}

func ReadFrames(r io.Reader) ([]Event, error) {
	reader := bufio.NewReader(r)
	events := make([]Event, 0)
	for {
		var header [4]byte
		_, err := io.ReadFull(reader, header[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("事件帧头截断: %w", err)
		}
		size := binary.BigEndian.Uint32(header[:])
		if size == 0 || size > MaxEventFrame {
			return nil, fmt.Errorf("事件帧长度不合法: %d", size)
		}
		raw := make([]byte, size)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return nil, fmt.Errorf("事件帧载荷截断: %w", err)
		}
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("事件帧 JSON 损坏: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}
