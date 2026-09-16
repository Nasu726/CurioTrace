package observation

import (
	"encoding/json"
)

const (
	SchemaVersion = "1.0"
	MaxEventBytes = 256 * 1024
)

type Source struct {
	URL          string `json:"url,omitempty"`
	Title        string `json:"title,omitempty"`
	CanonicalURL string `json:"canonical_url,omitempty"`
}

type Event struct {
	SchemaVersion     string         `json:"schema_version"`
	EventID           string         `json:"event_id"`
	SessionID         string         `json:"session_id"`
	RecordingEpoch    uint64         `json:"recording_epoch"`
	EventType         string         `json:"event_type"`
	WallTime          string         `json:"wall_time"`
	MonotonicMS       float64        `json:"monotonic_ms"`
	BrowserInstanceID string         `json:"browser_instance_id,omitempty"`
	ViewID            string         `json:"view_id,omitempty"`
	CaptureMode       string         `json:"capture_mode,omitempty"`
	Source            *Source        `json:"source,omitempty"`
	Payload           map[string]any `json:"payload"`
}

type SubmitPayload struct {
	Event json.RawMessage `json:"event"`
}
