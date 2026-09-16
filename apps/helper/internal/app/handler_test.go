package app

import (
	"encoding/json"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
)

func TestHandshakeAndLifecycle(t *testing.T) {
	h := NewHandler()
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
		MessageID: "p",
		Kind: "session.pause",
		SessionID: sessionID,
		RecordingEpoch: epoch,
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
		MessageID: "stale",
		Kind: "session.resume",
		SessionID: sessionID,
		RecordingEpoch: epoch,
	})
	if payloadMap(t, stale)["accepted"] != false {
		t.Fatal("stale control request unexpectedly accepted")
	}
}

func TestObservationPipelineFailsClosedUntilImplemented(t *testing.T) {
	h := NewHandler()
	start := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "session.start"})
	payload := payloadMap(t, start)
	sessionID := payload["session_id"].(string)
	epoch := uint64(payload["recording_epoch"].(float64))

	response := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind: "observation.submit",
		SessionID: sessionID,
		RecordingEpoch: epoch,
		Payload: json.RawMessage(`{"event":{"event_type":"content_observation"}}`),
	})
	result := payloadMap(t, response)
	if result["accepted"] != false || result["reason"] != "OBSERVATION_PIPELINE_NOT_IMPLEMENTED" {
		t.Fatalf("unexpected observation response: %#v", result)
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

func payloadMap(t *testing.T, message protocol.Envelope) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
