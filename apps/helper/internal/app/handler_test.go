package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

func TestHandshakeAndLifecycle(t *testing.T) {
	memory := store.NewMemoryStore()
	h := NewHandlerWithStore(memory)
	hello := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, MessageID: "h", Kind: "hello"})
	if hello.Kind != "hello.ack" {
		t.Fatalf("unexpected hello response: %+v", hello)
	}

	start := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, MessageID: "s", Kind: "session.start"})
	startPayload := payloadMap(t, start)
	if startPayload["accepted"] != true || startPayload["state"] != "RECORDING" {
		t.Fatalf("unexpected start response: %#v", startPayload)
	}
	sessionID, _ := startPayload["session_id"].(string)
	epoch := uint64(startPayload["recording_epoch"].(float64))

	pause := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       "p",
		Kind:            "session.pause",
		SessionID:       sessionID,
		RecordingEpoch:  epoch,
	})
	pausePayload := payloadMap(t, pause)
	if pausePayload["accepted"] != true || pausePayload["state"] != "PAUSED" {
		t.Fatalf("unexpected pause response: %#v", pausePayload)
	}
	pauseEpoch := uint64(pausePayload["recording_epoch"].(float64))
	if pauseEpoch == epoch {
		t.Fatal("pause did not advance recording epoch")
	}

	stale := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       "stale",
		Kind:            "session.resume",
		SessionID:       sessionID,
		RecordingEpoch:  epoch,
	})
	if payloadMap(t, stale)["accepted"] != false {
		t.Fatal("stale control request unexpectedly accepted")
	}
}

func TestValidObservationIsPersistedAfterValidation(t *testing.T) {
	memory := store.NewMemoryStore()
	h := NewHandlerWithStore(memory)
	sessionID, epoch := startSession(t, h)

	response := h.Handle(observationMessage(t, sessionID, epoch, map[string]any{
		"schema_version":  "1.0",
		"event_id":        "evt_1",
		"session_id":      sessionID,
		"recording_epoch": epoch,
		"event_type":      "content_observation",
		"wall_time":       "2026-09-16T00:00:00Z",
		"monotonic_ms":    10.0,
		"capture_mode":    "dom",
		"source":          map[string]any{"url": "https://example.test/article"},
		"payload": map[string]any{
			"units": []any{map[string]any{"text": "visible text"}},
		},
	}))
	result := payloadMap(t, response)
	if result["accepted"] != true || result["reason"] != "OK" {
		t.Fatalf("unexpected observation response: %#v", result)
	}

	events, err := memory.ListSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("unexpected persisted events: %#v", events)
	}
}

func TestPrivacyInvalidObservationNeverReachesStore(t *testing.T) {
	memory := store.NewMemoryStore()
	h := NewHandlerWithStore(memory)
	sessionID, epoch := startSession(t, h)

	response := h.Handle(observationMessage(t, sessionID, epoch, map[string]any{
		"schema_version":  "1.0",
		"event_id":        "evt_secret",
		"session_id":      sessionID,
		"recording_epoch": epoch,
		"event_type":      "content_observation",
		"wall_time":       "2026-09-16T00:00:00Z",
		"monotonic_ms":    11,
		"capture_mode":    "fingerprint_only",
		"payload": map[string]any{
			"visual_fingerprint": "abc",
			"ocr_text":           "must not persist",
		},
	}))
	result := payloadMap(t, response)
	if result["accepted"] != false {
		t.Fatalf("privacy-invalid event unexpectedly accepted: %#v", result)
	}

	events, err := memory.ListSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("privacy-invalid event reached store: %#v", events)
	}
}

func TestStateEventsRemainHelperAuthoritative(t *testing.T) {
	memory := store.NewMemoryStore()
	h := NewHandlerWithStore(memory)
	sessionID, epoch := startSession(t, h)

	response := h.Handle(observationMessage(t, sessionID, epoch, map[string]any{
		"schema_version":  "1.0",
		"event_id":        "evt_state",
		"session_id":      sessionID,
		"recording_epoch": epoch,
		"event_type":      "session_state",
		"wall_time":       "2026-09-16T00:00:00Z",
		"monotonic_ms":    12,
		"payload":         map[string]any{},
	}))
	result := payloadMap(t, response)
	if result["accepted"] != false || result["reason"] != "STATE_EVENT_HELPER_AUTHORITY" {
		t.Fatalf("unexpected state-event response: %#v", result)
	}
}

func TestDefaultHandlerRefusesStartWithoutStore(t *testing.T) {
	h := NewHandler()
	start := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "session.start"})
	result := payloadMap(t, start)
	if result["accepted"] != false || result["reason"] != "STORE_NOT_CONFIGURED" {
		t.Fatalf("helper without durable store unexpectedly started recording: %#v", result)
	}

	hello := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "hello"})
	helloPayload := payloadMap(t, hello)
	if helloPayload["session_state"] != "IDLE" {
		t.Fatalf("rejected start mutated helper state: %#v", helloPayload)
	}
}

func TestIncompatibleProtocolFailsClosed(t *testing.T) {
	h := NewHandler()
	response := h.Handle(protocol.Envelope{ProtocolVersion: "2.0", Kind: "session.start"})
	result := payloadMap(t, response)
	if result["accepted"] != false || result["reason"] != "INCOMPATIBLE_PROTOCOL" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func startSession(t *testing.T, h *Handler) (string, uint64) {
	t.Helper()
	start := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "session.start"})
	payload := payloadMap(t, start)
	if payload["accepted"] != true {
		t.Fatalf("failed to start session: %#v", payload)
	}
	return payload["session_id"].(string), uint64(payload["recording_epoch"].(float64))
}

func observationMessage(t *testing.T, sessionID string, epoch uint64, event map[string]any) protocol.Envelope {
	t.Helper()
	rawEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	rawPayload, err := json.Marshal(map[string]any{"event": json.RawMessage(rawEvent)})
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind:            "observation.submit",
		SessionID:       sessionID,
		RecordingEpoch:  epoch,
		Payload:         rawPayload,
	}
}

func payloadMap(t *testing.T, message protocol.Envelope) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
