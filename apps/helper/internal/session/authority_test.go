package session

import "testing"

func TestAuthorityLifecycleInvalidatesEpoch(t *testing.T) {
	a := NewAuthority()
	started, err := a.Start()
	if err != nil {
		t.Fatal(err)
	}
	if started.State != Recording || started.SessionID == "" || started.RecordingEpoch == 0 {
		t.Fatalf("unexpected start snapshot: %+v", started)
	}

	oldEpoch := started.RecordingEpoch
	paused, err := a.Pause(started.SessionID, oldEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if paused.State != Paused || paused.RecordingEpoch == oldEpoch {
		t.Fatalf("pause did not invalidate epoch: %+v", paused)
	}
	if a.AcceptsObservation(started.SessionID, oldEpoch) {
		t.Fatal("old recording epoch remained authorized after Pause")
	}

	resumed, err := a.Resume(paused.SessionID, paused.RecordingEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != Recording || resumed.RecordingEpoch == paused.RecordingEpoch {
		t.Fatalf("resume did not issue new epoch: %+v", resumed)
	}
	if !a.AcceptsObservation(resumed.SessionID, resumed.RecordingEpoch) {
		t.Fatal("current recording authority not accepted")
	}

	stopped, err := a.Stop(resumed.SessionID, resumed.RecordingEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != Finished || a.AcceptsObservation(stopped.SessionID, stopped.RecordingEpoch) {
		t.Fatalf("stop left capture authority active: %+v", stopped)
	}
}

func TestInterruptFailsClosed(t *testing.T) {
	a := NewAuthority()
	started, err := a.Start()
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := a.Interrupt()
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.State != Interrupted {
		t.Fatalf("expected interrupted, got %+v", interrupted)
	}
	if interrupted.RecordingEpoch == started.RecordingEpoch {
		t.Fatal("interrupt did not invalidate epoch")
	}
	if a.AcceptsObservation(started.SessionID, started.RecordingEpoch) {
		t.Fatal("pre-interrupt authority still accepted")
	}
}

func TestStaleTransitionRejected(t *testing.T) {
	a := NewAuthority()
	started, err := a.Start()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Pause(started.SessionID, started.RecordingEpoch+1); err == nil {
		t.Fatal("stale epoch transition unexpectedly accepted")
	}
	if a.Snapshot().State != Recording {
		t.Fatal("rejected transition mutated state")
	}
}
