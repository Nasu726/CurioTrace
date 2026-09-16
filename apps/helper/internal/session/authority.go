package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	State State
	SessionID string
	RecordingEpoch uint64
}

type Authority struct {
	mu sync.Mutex
	state State
	sessionID string
	recordingEpoch uint64
}

func NewAuthority() *Authority {
	return &Authority{state: Idle}
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
	a.sessionID = newID("ses_")
	a.recordingEpoch++
	a.state = Recording
	return a.snapshotLocked(), nil
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

func (a *Authority) Interrupt() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == Recording || a.state == Paused {
		a.recordingEpoch++
		a.state = Interrupted
	}
	return a.snapshotLocked()
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
	a.recordingEpoch++
	a.state = next
	return a.snapshotLocked(), nil
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
