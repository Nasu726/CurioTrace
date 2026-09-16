package app

import (
	"context"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

func TestDurableAuthorityCapabilityReflectsCurrentWiring(t *testing.T) {
	ephemeral := NewHandlerWithStore(store.NewMemoryStore())
	ephemeralHello := payloadMap(t, ephemeral.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind:            "hello",
	}))
	if hasCapability(ephemeralHello, "durable_session_authority_v1") {
		t.Fatal("ephemeral authority advertised durable_session_authority_v1")
	}

	repository := &handlerSnapshotRepository{}
	authority, err := session.OpenAuthority(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	durable := NewHandlerWithAuthorityAndStore(authority, store.NewMemoryStore())
	durableHello := payloadMap(t, durable.Handle(protocol.Envelope{
		ProtocolVersion: protocol.Version,
		Kind:            "hello",
	}))
	if !hasCapability(durableHello, "durable_session_authority_v1") {
		t.Fatal("durable authority did not advertise durable_session_authority_v1")
	}
}

func hasCapability(payload map[string]any, want string) bool {
	values, _ := payload["helper_capabilities"].([]any)
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
