package observation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodeAndValidateAcceptsSafeDOMObservation(t *testing.T) {
	validated, err := DecodeAndValidate(validEventJSON(`"capture_mode":"dom","source":{"url":"https://example.test/"},"payload":{"units":[{"text":"visible article text"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if validated.SessionID() != "ses_1" || validated.RecordingEpoch() != 7 || validated.EventType() != "content_observation" {
		t.Fatalf("unexpected validated event: session=%q epoch=%d type=%q", validated.SessionID(), validated.RecordingEpoch(), validated.EventType())
	}
}

func TestBlockedObservationRejectsSourceAndContent(t *testing.T) {
	_, err := DecodeAndValidate(validEventJSON(`"capture_mode":"blocked","source":{"url":"https://secret.test/"},"payload":{"reason":"excluded","text":"must-not-persist"}`))
	assertValidationContains(t, err, "BLOCKED_SOURCE_FORBIDDEN")
	assertValidationContains(t, err, "FORBIDDEN_PAYLOAD_FIELD")
}

func TestFingerprintOnlyRejectsContent(t *testing.T) {
	_, err := DecodeAndValidate(validEventJSON(`"capture_mode":"fingerprint_only","payload":{"visual_fingerprint":"abc","ocr_text":"secret"}`))
	assertValidationContains(t, err, "FINGERPRINT_ONLY_CONTENT_FORBIDDEN")
	assertValidationContains(t, err, "FORBIDDEN_PAYLOAD_FIELD:ocr_text")
}

func TestRedactedVisualRequiresPolicyVersion(t *testing.T) {
	_, err := DecodeAndValidate(validEventJSON(`"capture_mode":"redacted_visual","payload":{"units":[]}`))
	assertValidationContains(t, err, "REDACTION_POLICY_REQUIRED")
}

func TestUnknownTopLevelFieldRejected(t *testing.T) {
	raw := validEventJSON(`"capture_mode":"metadata_only","payload":{"reason":"test"},"surprise":"nope"`)
	_, err := DecodeAndValidate(raw)
	assertValidationContains(t, err, "UNKNOWN_TOP_LEVEL_FIELD:surprise")
}

func TestUnknownSourceFieldRejected(t *testing.T) {
	raw := validEventJSON(`"capture_mode":"dom","source":{"url":"https://example.test/","secret":"nope"},"payload":{"units":[]}`)
	_, err := DecodeAndValidate(raw)
	assertValidationContains(t, err, "INVALID_SCHEMA")
}

func TestMissingAndNullRequiredFieldsRejected(t *testing.T) {
	missing := json.RawMessage(`{"schema_version":"1.0","event_id":"evt_1","session_id":"ses_1","recording_epoch":7,"event_type":"content_observation","wall_time":"2026-09-16T00:00:00Z","payload":{}}`)
	_, err := DecodeAndValidate(missing)
	assertValidationContains(t, err, "MISSING_FIELD:monotonic_ms")

	nullValue := json.RawMessage(`{"schema_version":"1.0","event_id":"evt_1","session_id":"ses_1","recording_epoch":null,"event_type":"content_observation","wall_time":"2026-09-16T00:00:00Z","monotonic_ms":1,"payload":{}}`)
	_, err = DecodeAndValidate(nullValue)
	assertValidationContains(t, err, "NULL_REQUIRED_FIELD:recording_epoch")
}

func TestEditableValueRejectedEvenNested(t *testing.T) {
	raw := validEventJSON(`"capture_mode":"dom","payload":{"units":[{"text":"ok","meta":{"input_value":"typed secret"}}]}`)
	_, err := DecodeAndValidate(raw)
	assertValidationContains(t, err, "FORBIDDEN_PAYLOAD_FIELD:input_value")
}

func validEventJSON(extra string) json.RawMessage {
	base := `{"schema_version":"1.0","event_id":"evt_1","session_id":"ses_1","recording_epoch":7,"event_type":"content_observation","wall_time":"2026-09-16T00:00:00Z","monotonic_ms":123.5,`
	return json.RawMessage(base + extra + `}`)
}

func assertValidationContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error containing %q", want)
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	for _, code := range validation.Codes {
		if code == want || strings.HasPrefix(code, want) {
			return
		}
	}
	t.Fatalf("validation codes %v do not contain %q", validation.Codes, want)
}
