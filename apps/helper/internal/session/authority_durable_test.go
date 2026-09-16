package session

import (
	"context"
	"errors"
	"sync"
	"testing"
)

var errTestSnapshotSave = errors.New("snapshot save failed")

type fakeSnapshotRepository struct {
	mu       sync.Mutex
	snapshot Snapshot
	found    bool
	loadErr  error
	saveErr  error
	saves    []Snapshot
}

func (r *fakeSnapshotRepository) Load(_ context.Context) (Snapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot, r.found, r.loadErr
}

func (r *fakeSnapshotRepository) Save(_ context.Context, snapshot Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves = append(r.saves, snapshot)
	if r.saveErr != nil {
		return r.saveErr
	}
	r.snapshot = snapshot
	r.found = true
	return nil
}

func TestOpenAuthorityConvertsUnfinishedStateToInterrupted(t *testing.T) {
	for _, state := range []State{Recording, Paused} {
		t.Run(string(state), func(t *testing.T) {
			repo := &fakeSnapshotRepository{
				snapshot: Snapshot{State: state, SessionID: testSessionID("a"), RecordingEpoch: 7},
				found:    true,
			}
			authority, err := OpenAuthority(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			got := authority.Snapshot()
			if got.State != Interrupted || got.SessionID != testSessionID("a") || got.RecordingEpoch != 8 {
				t.Fatalf("unfinished state did not recover as interrupted: %+v", got)
			}
			if len(repo.saves) != 1 || repo.saves[0] != got {
				t.Fatalf("startup interruption was not durably committed first: %#v", repo.saves)
			}
			if authority.AcceptsObservation(got.SessionID, 7) || authority.AcceptsObservation(got.SessionID, 8) {
				t.Fatal("recovered interrupted authority accepted observation")
			}

			// Reopening an already interrupted snapshot must not keep incrementing
			// the epoch merely because the helper process restarted again.
			second, err := OpenAuthority(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if second.Snapshot() != got || len(repo.saves) != 1 {
				t.Fatalf("already interrupted snapshot changed on second reopen: got=%+v saves=%#v", second.Snapshot(), repo.saves)
			}
		})
	}
}

func TestOpenAuthorityRequiresDurableStartupInterruption(t *testing.T) {
	repo := &fakeSnapshotRepository{
		snapshot: Snapshot{State: Recording, SessionID: testSessionID("b"), RecordingEpoch: 11},
		found:    true,
		saveErr:  errTestSnapshotSave,
	}
	if _, err := OpenAuthority(context.Background(), repo); !errors.Is(err, ErrStatePersistence) {
		t.Fatalf("startup exposed authority despite failed interruption persistence: %v", err)
	}
}

func TestOpenAuthorityPreservesFinishedState(t *testing.T) {
	want := Snapshot{State: Finished, SessionID: testSessionID("c"), RecordingEpoch: 5}
	repo := &fakeSnapshotRepository{snapshot: want, found: true}
	authority, err := OpenAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := authority.Snapshot(); got != want {
		t.Fatalf("finished snapshot changed on reopen: got=%+v want=%+v", got, want)
	}
	if len(repo.saves) != 0 {
		t.Fatalf("finished state was unnecessarily rewritten: %#v", repo.saves)
	}
}

func TestStartPersistenceFailureDoesNotCreateCaptureAuthority(t *testing.T) {
	repo := &fakeSnapshotRepository{saveErr: errTestSnapshotSave}
	authority, err := OpenAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Start(); !errors.Is(err, ErrStatePersistence) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	if got := authority.Snapshot(); got.State != Idle || got.SessionID != "" || got.RecordingEpoch != 0 {
		t.Fatalf("failed Start mutated authority: %+v", got)
	}
}

func TestPausePersistenceFailureRevokesInMemoryRecording(t *testing.T) {
	repo := &fakeSnapshotRepository{}
	authority, err := OpenAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	started, err := authority.Start()
	if err != nil {
		t.Fatal(err)
	}
	repo.saveErr = errTestSnapshotSave

	got, err := authority.Pause(started.SessionID, started.RecordingEpoch)
	if !errors.Is(err, ErrStatePersistence) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	if got.State != Interrupted || got.RecordingEpoch == started.RecordingEpoch {
		t.Fatalf("failed Pause left unsafe state: %+v", got)
	}
	if authority.AcceptsObservation(started.SessionID, started.RecordingEpoch) || authority.AcceptsObservation(got.SessionID, got.RecordingEpoch) {
		t.Fatal("failed Pause left capture authority active")
	}
}

func TestResumePersistenceFailureStaysNonRecording(t *testing.T) {
	repo := &fakeSnapshotRepository{}
	authority, err := OpenAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	started, err := authority.Start()
	if err != nil {
		t.Fatal(err)
	}
	paused, err := authority.Pause(started.SessionID, started.RecordingEpoch)
	if err != nil {
		t.Fatal(err)
	}
	repo.saveErr = errTestSnapshotSave

	got, err := authority.Resume(paused.SessionID, paused.RecordingEpoch)
	if !errors.Is(err, ErrStatePersistence) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	if got != paused || got.State != Paused {
		t.Fatalf("failed Resume changed non-recording authority: got=%+v paused=%+v", got, paused)
	}
}

func TestInterruptPersistenceFailureStillRevokesMemoryAuthority(t *testing.T) {
	repo := &fakeSnapshotRepository{}
	authority, err := OpenAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	started, err := authority.Start()
	if err != nil {
		t.Fatal(err)
	}
	repo.saveErr = errTestSnapshotSave

	interrupted, err := authority.Interrupt()
	if !errors.Is(err, ErrStatePersistence) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	if interrupted.State != Interrupted || interrupted.RecordingEpoch == started.RecordingEpoch {
		t.Fatalf("Interrupt did not revoke memory authority: %+v", interrupted)
	}
	if authority.AcceptsObservation(started.SessionID, started.RecordingEpoch) {
		t.Fatal("pre-interrupt epoch remained authorized")
	}
}

func testSessionID(hexDigit string) string {
	id := "ses_"
	for len(id) < len("ses_")+32 {
		id += hexDigit
	}
	return id
}
