package store

import (
	"context"
	"sync"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string][]observation.ValidatedEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string][]observation.ValidatedEvent)}
}

func (s *MemoryStore) Ready(ctx context.Context) error {
	return ctx.Err()
}

func (s *MemoryStore) Append(_ context.Context, event observation.ValidatedEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[event.SessionID()] = append(s.sessions[event.SessionID()], event)
	return nil
}

func (s *MemoryStore) ListSession(_ context.Context, sessionID string) ([]observation.ValidatedEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.sessions[sessionID]
	copyOfEvents := make([]observation.ValidatedEvent, len(events))
	copy(copyOfEvents, events)
	return copyOfEvents, nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}
