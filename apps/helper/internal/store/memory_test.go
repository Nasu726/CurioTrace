package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

func TestMemoryStorePersistsOnlyValidatedEvents(t *testing.T) {
	validated, err := observation.DecodeAndValidate(json.RawMessage(`{"schema_version":"1.0","event_id":"evt_1","session_id":"ses_1","recording_epoch":1,"event_type":"content_observation","wall_time":"2026-09-16T00:00:00Z","monotonic_ms":1,"capture_mode":"metadata_only","payload":{"reason":"test"}}`))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	memory := NewMemoryStore()
	if err := memory.Append(ctx, validated); err != nil {
		t.Fatal(err)
	}
	events, err := memory.ListSession(ctx, "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("unexpected events: %#v", events)
	}

	if err := memory.DeleteSession(ctx, "ses_1"); err != nil {
		t.Fatal(err)
	}
	events, err = memory.ListSession(ctx, "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("session delete left %d events", len(events))
	}
}
