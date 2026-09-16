package store

import (
	"context"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

// Store persists only events that crossed the observation validator boundary.
// Raw protocol payloads and rejected events are intentionally absent from this API.
type Store interface {
	Append(ctx context.Context, event observation.ValidatedEvent) error
	ListSession(ctx context.Context, sessionID string) ([]observation.ValidatedEvent, error)
	DeleteSession(ctx context.Context, sessionID string) error
}
