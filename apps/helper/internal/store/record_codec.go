package store

import "github.com/Nasu726/CurioTrace/apps/helper/internal/observation"

// RecordCodec transforms a validated durable event into the bytes stored in a
// session log and back again. Production wiring must provide a codec that meets
// the product's at-rest protection requirements; FileStore deliberately does
// not choose or weaken that policy itself.
type RecordCodec interface {
	ID() string
	Encode(event observation.ValidatedEvent) ([]byte, error)
	Decode(record []byte) (observation.ValidatedEvent, error)
}
