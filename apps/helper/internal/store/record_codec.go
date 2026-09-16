package store

import (
	"context"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

// RecordCodec transforms a validated durable event into the bytes stored in a
// session log and back again. Production wiring must provide a codec that meets
// the product's at-rest protection requirements; FileStore deliberately does
// not choose or weaken that policy itself.
//
// Context is part of the boundary because production codecs may obtain key
// material from platform services that can block or be cancelled.
type RecordCodec interface {
	ID() string
	Encode(ctx context.Context, event observation.ValidatedEvent) ([]byte, error)
	Decode(ctx context.Context, record []byte) (observation.ValidatedEvent, error)
}
