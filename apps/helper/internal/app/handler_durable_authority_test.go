package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

var errHandlerSnapshotSave = errors.New("handler snapshot save failed")

type handlerSnapshotRepository struct {
	mu       sync.Mutex
	snapshot session.Snapshot
	found    bool
	saveErr  error
}

func (r *handlerSnapshotRepository) Load(_ context.Context) (session.Snapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot, r.found, nil
}

func (r *handlerSnapshotRepository) Save(_ context.Context, snapshot session.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return r.saveErr
	}
	r.snapshot = snapshot
	r.found = true
	return nil
}

func TestHandlerPausePersistenceFailureReturnsInterruptedAuthority(t *testing.T) {
	repository := &handlerSnapshotRepository{}
	authority, err := session.OpenAuthority(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithAuthorityAndStore(authority, store.NewMemoryStore())
	sessionID, epoch := startSession(t, h)

	repository.mu.Lock()
	repository.saveErr = errHandlerSnapshotSave
	repository.mu.Unlock()

	response := h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       "pause-fail",
		Kind:            "session.pause",
		SessionID:       sessionID,
		RecordingEpoch:  epoch,
	})
	payload := payloadMap(t, response)
	if payload["accepted"] != false || payload["reason"] != "STATE_PERSISTENCE_ERROR" {
		t.Fatalf("unexpected failed Pause response: %#v", payload)
	}
	if payload["state"] != "INTERRUPTED" {
		t.Fatalf("failed Pause did not expose interrupted state: %#v", payload)
	}
	interruptedEpoch := uint64(payload["recording_epoch"].(float64))
	if interruptedEpoch == epoch {
		t.Fatalf("failed Pause did not invalidate epoch: %#v", payload)
	}

	hello := payloadMap(t, h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       "hello-after-fail",
		Kind:            "hello",
	}))
	if hello["session_state"] != "INTERRUPTED" || uint64(hello["recording_epoch"].(float64)) != interruptedEpoch {
		t.Fatalf("hello did not expose fail-closed authority: %#v", hello)
	}
}

func TestHandlerHelloExposesRecoveredInterruptedAuthority(t *testing.T) {
	repository := &handlerSnapshotRepository{
		found: true,
		snapshot: session.Snapshot{
			State:          session.Recording,
			SessionID:      "ses_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RecordingEpoch: 9,
		},
	}
	authority, err := session.OpenAuthority(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithAuthorityAndStore(authority, store.NewMemoryStore())

	hello := payloadMap(t, h.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       "hello-recovered",
		Kind:            "hello",
	}))
	if hello["session_state"] != "INTERRUPTED" || hello["session_id"] != "ses_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("recovered authority not exposed by hello: %#v", hello)
	}
	if got := uint64(hello["recording_epoch"].(float64)); got != 10 {
		t.Fatalf("restart did not issue fresh epoch: got=%d payload=%#v", got, hello)
	}
}
