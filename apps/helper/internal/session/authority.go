package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

type State string

const (
	Idle        State = "IDLE"
	Recording   State = "RECORDING"
	Paused      State = "PAUSED"
	Finished    State = "FINISHED"
	Interrupted State = "INTERRUPTED"
)

var ErrInvalidTransition = errors.New("invalid session state transition")

type Snapshot struct {
	State          State
	SessionID      string
	RecordingEpoch uint64
}

type Authority struct {
	mu             sync.Mutex
	state          State
	sessionID      string
	recordingEpoch uint64
	repository     SnapshotRepository
}

// NewAuthority creates an in-memory authority for tests/reference wiring. Normal
// production recording must use OpenAuthority with a durable repository.
func NewAuthority() *Authority {
	return &Authority{state: Idle}
}

// OpenAuthority restores helper-authoritative control state. An unfinished
// RECORDING/PAUSED snapshot is never resumed implicitly: startup first commits
// INTERRUPTED with a fresh epoch and only then exposes the authority.
func OpenAuthority(ctx context.Context, repository SnapshotRepository) (*Authority, error) {
	if repository == nil {
		return nil, ErrRepositoryMissing
	}
	snapshot, found, err := repository.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load durable session authority: %w", err)
	}
	if !found {
		return &Authority{state: Idle, repository: repository}, nil
	}
	if err := validateSnapshot(snapshot); err != nil {
		return nil, err
	}
	if snapshot.State == Recording || snapshot.State == Paused {
		epoch, err := nextEpoch(snapshot.RecordingEpoch)
		if err != nil {
			return nil, err
		}
		snapshot.State = Interrupted
		snapshot.RecordingEpoch = epoch
		if err := repository.Save(ctx, snapshot); err != nil {
			return nil, fmt.Errorf("%w: persist startup interruption: %v", ErrStatePersistence, err)
		}
	}
	return &Authority{
		state:          snapshot.State,
		sessionID:      snapshot.SessionID,
		recordingEpoch: snapshot.RecordingEpoch,
		repository:     repository,
	}, nil
}

func (a *Authority) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *Authority) Start() (Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != Idle && a.state != Finished {
		return a.snapshotLocked(), ErrInvalidTransition
	}
	epoch, err := nextEpoch(a.recordingEpoch)
	if err != nil {
		return a.snapshotLocked(), err
	}
	candidate := Snapshot{
		State:          Recording,
		SessionID:      newID("ses_"),
		RecordingEpoch: epoch,
	}
	if err := a.persistLocked(candidate); err != nil {
		return a.snapshotLocked(), err
	}
	a.applyLocked(candidate)
	return candidate, nil
}

func (a *Authority) Pause(sessionID string, epoch uint64) (Snapshot, error) {
	return a.transition(sessionID, epoch, []State{Recording}, Paused)
}

func (a *Authority) Resume(sessionID string, epoch uint64) (Snapshot, error) {
	return a.transition(sessionID, epoch, []State{Paused, Interrupted}, Recording)
}

func (a *Authority) Stop(sessionID string, epoch uint64) (Snapshot, error) {
	return a.transition(sessionID, epoch, []State{Recording, Paused, Interrupted}, Finished)
}

// Interrupt revokes in-memory capture authority before attempting durability.
// If persistence is unavailable, the current process remains fail-closed; a
// stale durable RECORDING/PAUSED snapshot is converted to INTERRUPTED again on
// the next OpenAuthority startup path.
func (a *Authority) Interrupt() (Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != Recording && a.state != Paused {
		return a.snapshotLocked(), nil
	}
	epoch, err := nextEpoch(a.recordingEpoch)
	if err != nil {
		// State alone is sufficient to revoke AcceptsObservation even in the
		// practically unreachable epoch-exhaustion case.
		a.state = Interrupted
		return a.snapshotLocked(), err
	}
	candidate := Snapshot{
		State:          Interrupted,
		SessionID:      a.sessionID,
		RecordingEpoch: epoch,
	}
	a.applyLocked(candidate)
	if err := a.persistLocked(candidate); err != nil {
		return candidate, err
	}
	return candidate, nil
}

func (a *Authority) AcceptsObservation(sessionID string, epoch uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state == Recording && a.sessionID == sessionID && a.recordingEpoch == epoch
}

func (a *Authority) transition(sessionID string, epoch uint64, allowed []State, next State) (Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionID != sessionID || a.recordingEpoch != epoch || !contains(allowed, a.state) {
		return a.snapshotLocked(), ErrInvalidTransition
	}
	nextRecordingEpoch, err := nextEpoch(a.recordingEpoch)
	if err != nil {
		return a.snapshotLocked(), err
	}
	candidate := Snapshot{
		State:          next,
		SessionID:      a.sessionID,
		RecordingEpoch: nextRecordingEpoch,
	}
	if err := a.persistLocked(candidate); err != nil {
		// A failed transition *out of RECORDING* must not leave capture authority
		// active merely because the state journal is unavailable. Revoke memory
		// authority immediately and best-effort persist that safer state.
		if a.state == Recording && next != Recording {
			a.interruptAfterPersistenceFailureLocked()
		}
		return a.snapshotLocked(), err
	}
	a.applyLocked(candidate)
	return candidate, nil
}

func (a *Authority) interruptAfterPersistenceFailureLocked() {
	epoch, err := nextEpoch(a.recordingEpoch)
	if err != nil {
		a.state = Interrupted
		return
	}
	candidate := Snapshot{
		State:          Interrupted,
		SessionID:      a.sessionID,
		RecordingEpoch: epoch,
	}
	a.applyLocked(candidate)
	_ = a.persistLocked(candidate)
}

func (a *Authority) persistLocked(snapshot Snapshot) error {
	if a.repository == nil {
		return nil
	}
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if err := a.repository.Save(context.Background(), snapshot); err != nil {
		return fmt.Errorf("%w: %v", ErrStatePersistence, err)
	}
	return nil
}

func (a *Authority) applyLocked(snapshot Snapshot) {
	a.state = snapshot.State
	a.sessionID = snapshot.SessionID
	a.recordingEpoch = snapshot.RecordingEpoch
}

func (a *Authority) snapshotLocked() Snapshot {
	return Snapshot{State: a.state, SessionID: a.sessionID, RecordingEpoch: a.recordingEpoch}
}

func contains(states []State, value State) bool {
	for _, state := range states {
		if state == value {
			return true
		}
	}
	return false
}

func newID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + hex.EncodeToString(raw[:])
}
