package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
)

const testSessionID = "ses_0123456789abcdef0123456789abcdef"

type fakeReader struct {
	events []observation.ValidatedEvent
	err    error
	calls  int
}

func (f *fakeReader) ListSession(_ context.Context, sessionID string) ([]observation.ValidatedEvent, error) {
	f.calls++
	if sessionID != testSessionID {
		return nil, errors.New("unexpected session id")
	}
	if f.err != nil {
		return nil, f.err
	}
	return append([]observation.ValidatedEvent(nil), f.events...), nil
}

func testPaths() (platformpath.Paths, error) {
	return platformpath.Paths{
		Root:         "/tmp/curiotrace-test",
		Observations: "/tmp/curiotrace-test/observations",
		Authority:    "/tmp/curiotrace-test/authority",
	}, nil
}

func TestStatusReportsFailClosedProductionReadiness(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"status"}, &out, &errOut, Dependencies{
		GOOS:         "linux",
		ResolvePaths: testPaths,
		Readiness: func(context.Context) Readiness {
			return Readiness{
				RecordingAvailable:  false,
				RecordingReason:     ReasonBootstrapUnavailable,
				InspectionAvailable: false,
				InspectionReason:    ReasonInspectUnavailable,
			}
		},
	})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{
		"platform: linux",
		"recording bootstrap: unavailable (PRODUCTION_BOOTSTRAP_UNAVAILABLE)",
		"session inspection: unavailable (SESSION_INSPECTION_UNAVAILABLE)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestStatusJSONIsMachineReadable(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"status", "--json"}, &out, &errOut, Dependencies{
		GOOS:         "linux",
		ResolvePaths: testPaths,
		Readiness: func(context.Context) Readiness {
			return Readiness{RecordingAvailable: true, InspectionAvailable: true}
		},
	})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	var report StatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Platform != "linux" || !report.Readiness.RecordingAvailable || !report.Readiness.InspectionAvailable {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestInspectRequiresValidProductionSessionIDBeforeReader(t *testing.T) {
	reader := &fakeReader{}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"inspect", "--session", "../secret"}, &out, &errOut, Dependencies{Reader: reader})
	if code != ExitUsage {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if reader.calls != 0 {
		t.Fatalf("reader called %d times", reader.calls)
	}
}

func TestInspectFailsExplicitlyWhenEncryptedReaderIsUnavailable(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"inspect", "--session", testSessionID}, &out, &errOut, Dependencies{})
	if code != ExitUnavailable {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), ReasonInspectUnavailable) {
		t.Fatalf("missing unavailable reason: %q", errOut.String())
	}
}

func TestInspectHumanOutputUsesValidatedEvents(t *testing.T) {
	event := mustValidated(t, `{
		"schema_version":"1.0",
		"event_id":"evt_1",
		"session_id":"`+testSessionID+`",
		"recording_epoch":7,
		"event_type":"content_observation",
		"wall_time":"2026-09-18T00:00:00Z",
		"monotonic_ms":123.5,
		"capture_mode":"dom",
		"source":{"url":"https://example.test/article","title":"Example article"},
		"payload":{"units":[{"text":"visible article text"}]}
	}`)
	reader := &fakeReader{events: []observation.ValidatedEvent{event}}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"inspect", "--session", testSessionID}, &out, &errOut, Dependencies{Reader: reader})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{
		"Session " + testSessionID,
		"events: 1",
		"content_observation",
		"title: Example article",
		"url: https://example.test/article",
		"visible article text",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestInspectJSONPreservesValidatedEventShape(t *testing.T) {
	event := mustValidated(t, `{
		"schema_version":"1.0",
		"event_id":"evt_2",
		"session_id":"`+testSessionID+`",
		"recording_epoch":8,
		"event_type":"visibility",
		"wall_time":"2026-09-18T00:00:01Z",
		"monotonic_ms":124.5,
		"capture_mode":"metadata_only",
		"payload":{"reason":"tab_active"}
	}`)
	reader := &fakeReader{events: []observation.ValidatedEvent{event}}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"inspect", "--session", testSessionID, "--json"}, &out, &errOut, Dependencies{Reader: reader})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	var decoded []map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0]["event_id"] != "evt_2" || decoded[0]["event_type"] != "visibility" {
		t.Fatalf("unexpected JSON: %#v", decoded)
	}
}

func TestInspectReaderFailureDoesNotEchoEventPayload(t *testing.T) {
	reader := &fakeReader{err: errors.New("durable store unavailable")}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"inspect", "--session", testSessionID}, &out, &errOut, Dependencies{Reader: reader})
	if code != ExitFailure {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "durable store unavailable") {
		t.Fatalf("missing safe error class: %q", errOut.String())
	}
}

func mustValidated(t *testing.T, raw string) observation.ValidatedEvent {
	t.Helper()
	event, err := observation.DecodeAndValidate(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return event
}
