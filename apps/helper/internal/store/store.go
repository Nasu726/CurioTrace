package store

import (
	"context"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

// Store persists only events that crossed the observation validator boundary.
// Raw protocol payloads and rejected events are intentionally absent from this API.
// Ready must fail when the configured durable path cannot safely accept new
// observations, so recording never starts on a known-unavailable store/key path.
type Store interface {
	Ready(ctx context.Context) error
	Append(ctx context.Context, event observation.ValidatedEvent) error
	ListSession(ctx context.Context, sessionID string) ([]observation.ValidatedEvent, error)
	DeleteSession(ctx context.Context, sessionID string) error
}
