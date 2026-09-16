package session

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrInvalidSnapshot   = errors.New("invalid durable session snapshot")
	ErrStatePersistence  = errors.New("durable session state persistence failed")
	ErrEpochExhausted    = errors.New("recording epoch exhausted")
	ErrRepositoryMissing = errors.New("session snapshot repository required")
)

// SnapshotRepository persists only helper-authoritative session control state.
// It must never contain browsing content, URLs, screenshots, OCR text, or other
// observation payloads.
type SnapshotRepository interface {
	Load(ctx context.Context) (Snapshot, bool, error)
	Save(ctx context.Context, snapshot Snapshot) error
}

func validateSnapshot(snapshot Snapshot) error {
	switch snapshot.State {
	case Idle:
		if snapshot.SessionID != "" || snapshot.RecordingEpoch != 0 {
			return fmt.Errorf("%w: IDLE must not carry session authority", ErrInvalidSnapshot)
		}
		return nil
	case Recording, Paused, Finished, Interrupted:
		if !validSessionID(snapshot.SessionID) || snapshot.RecordingEpoch == 0 {
			return fmt.Errorf("%w: active/history state requires session id and epoch", ErrInvalidSnapshot)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown state %q", ErrInvalidSnapshot, snapshot.State)
	}
}

func nextEpoch(epoch uint64) (uint64, error) {
	if epoch == math.MaxUint64 {
		return 0, ErrEpochExhausted
	}
	return epoch + 1, nil
}

func validSessionID(sessionID string) bool {
	if len(sessionID) != len("ses_")+32 || !strings.HasPrefix(sessionID, "ses_") {
		return false
	}
	for _, ch := range sessionID[len("ses_"):] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
