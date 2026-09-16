package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
)

type switchableStore struct {
	readyErr  error
	appendErr error
}

func (s *switchableStore) Ready(context.Context) error { return s.readyErr }
func (s *switchableStore) Append(context.Context, observation.ValidatedEvent) error {
	return s.appendErr
}
func (s *switchableStore) ListSession(context.Context, string) ([]observation.ValidatedEvent, error) {
	return nil, nil
}
func (s *switchableStore) DeleteSession(context.Context, string) error { return nil }

func TestResumeRefusesUnavailableStoreAndRemainsPaused(t *testing.T) {
	durable := &switchableStore{}
	h := NewHandlerWithStore(durable)
	sessionID, epoch := startSession(t, h)

	pause := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind:            "session.pause",
		SessionID:       sessionID,
		RecordingEpoch:  epoch,
	})
	pausePayload := payloadMap(t, pause)
	if pausePayload["accepted"] != true || pausePayload["state"] != "PAUSED" {
		t.Fatalf("failed to pause test session: %#v", pausePayload)
	}
	pausedEpoch := uint64(pausePayload["recording_epoch"].(float64))

	durable.readyErr = errors.New("key service unavailable")
	resume := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind:            "session.resume",
		SessionID:       sessionID,
		RecordingEpoch:  pausedEpoch,
	})
	resumePayload := payloadMap(t, resume)
	if resumePayload["accepted"] != false || resumePayload["reason"] != "STORE_UNAVAILABLE" {
		t.Fatalf("resume unexpectedly accepted unavailable store: %#v", resumePayload)
	}

	hello := payloadMap(t, h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "hello"}))
	if hello["session_state"] != "PAUSED" || uint64(hello["recording_epoch"].(float64)) != pausedEpoch {
		t.Fatalf("failed Resume mutated capture authority: %#v", hello)
	}
}

func TestAppendFailureInterruptsRecordingAuthority(t *testing.T) {
	durable := &switchableStore{}
	h := NewHandlerWithStore(durable)
	sessionID, epoch := startSession(t, h)
	durable.appendErr = errors.New("disk/key failure")

	response := h.Handle(observationMessage(t, sessionID, epoch, map[string]any{
		"schema_version":  "1.0",
		"event_id":        "evt_store_failure",
		"session_id":      sessionID,
		"recording_epoch": epoch,
		"event_type":      "content_observation",
		"wall_time":       "2026-09-16T00:00:00Z",
		"monotonic_ms":    1,
		"capture_mode":    "metadata_only",
		"payload":         map[string]any{"reason": "test"},
	}))
	result := payloadMap(t, response)
	if result["accepted"] != false || result["reason"] != "STORE_ERROR" || result["state"] != "INTERRUPTED" {
		t.Fatalf("store failure did not interrupt helper authority: %#v", result)
	}
	interruptedEpoch := uint64(result["recording_epoch"].(float64))
	if interruptedEpoch == epoch {
		t.Fatal("store failure did not invalidate recording epoch")
	}

	hello := payloadMap(t, h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "hello"}))
	if hello["session_state"] != "INTERRUPTED" || uint64(hello["recording_epoch"].(float64)) != interruptedEpoch {
		t.Fatalf("helper did not retain interrupted authority: %#v", hello)
	}
}
