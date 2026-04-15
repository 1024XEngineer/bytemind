package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type jsonlSnapshotEnvelope struct {
	Version   int             `json:"v"`
	Timestamp time.Time       `json:"ts"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func WriteJSONLSnapshot(files *SessionFileStore, path, eventType string, schemaVersion int, payload any, at time.Time) error {
	if files == nil {
		return errors.New("session file store is required")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("snapshot path is required")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return errors.New("snapshot event type is required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	record := jsonlSnapshotEnvelope{
		Version:   schemaVersion,
		Timestamp: at,
		Type:      eventType,
		Payload:   rawPayload,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return err
	}
	content := bytes.TrimSpace(buf.Bytes())
	content = append(content, '\n')
	return files.WriteAtomic(path, content)
}

func ReadLatestJSONLSnapshot(files *SessionFileStore, path, eventType string) (json.RawMessage, error) {
	if files == nil {
		return nil, errors.New("session file store is required")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("snapshot path is required")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return nil, errors.New("snapshot event type is required")
	}

	data, err := files.Read(path)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		var envelope jsonlSnapshotEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}
		if envelope.Type != eventType {
			continue
		}
		payload := bytes.TrimSpace(envelope.Payload)
		if len(payload) == 0 {
			continue
		}
		return json.RawMessage(payload), nil
	}
	return nil, errors.New("no valid session snapshot found")
}
