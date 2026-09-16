package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
)

var errTestStoreUnavailable = errors.New("test durable store unavailable")

type unavailableStore struct{}

func (unavailableStore) Ready(context.Context) error { return errTestStoreUnavailable }
func (unavailableStore) Append(context.Context, observation.ValidatedEvent) error {
	return errTestStoreUnavailable
}
func (unavailableStore) ListSession(context.Context, string) ([]observation.ValidatedEvent, error) {
	return nil, errTestStoreUnavailable
}
func (unavailableStore) DeleteSession(context.Context, string) error { return errTestStoreUnavailable }

func TestStartRefusesUnavailableDurableStoreWithoutMutatingAuthority(t *testing.T) {
	h := NewHandlerWithStore(unavailableStore{})
	start := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "session.start"})
	result := payloadMap(t, start)
	if result["accepted"] != false || result["reason"] != "STORE_UNAVAILABLE" {
		t.Fatalf("unavailable durable store unexpectedly started recording: %#v", result)
	}

	hello := h.Handle(protocol.Envelope{ProtocolVersion: protocol.Version, Kind: "hello"})
	helloPayload := payloadMap(t, hello)
	if helloPayload["session_state"] != "IDLE" || helloPayload["session_id"] != nil {
		t.Fatalf("failed Start mutated helper authority: %#v", helloPayload)
	}
}
