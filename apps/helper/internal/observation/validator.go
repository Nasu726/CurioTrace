package observation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrInvalidObservation = errors.New("invalid observation")

type ValidationError struct {
	Codes []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%v: %s", ErrInvalidObservation, strings.Join(e.Codes, ","))
}

func (e *ValidationError) Unwrap() error { return ErrInvalidObservation }

var eventTypes = setOf(
	"session_state",
	"navigation",
	"visibility",
	"content_observation",
	"interaction",
	"privacy_decision",
	"capture_status",
	"gap_error",
)

var captureModes = setOf(
	"dom",
	"redacted_visual",
	"fingerprint_only",
	"metadata_only",
	"blocked",
	"failed",
)

var requiredTopLevelFields = setOf(
	"schema_version",
	"event_id",
	"session_id",
	"recording_epoch",
	"event_type",
	"wall_time",
	"monotonic_ms",
	"payload",
)

var allowedTopLevelFields = setOf(
	"schema_version",
	"event_id",
	"session_id",
	"recording_epoch",
	"event_type",
	"wall_time",
	"monotonic_ms",
	"browser_instance_id",
	"view_id",
	"capture_mode",
	"source",
	"payload",
)

var forbiddenPayloadKeys = setOf(
	"form_value",
	"input_value",
	"textarea_value",
	"contenteditable_value",
	"password_value",
	"auth_token",
	"payment_value",
	"security_code",
	"clipboard",
	"clipboard_text",
	"image",
	"image_bytes",
	"raster",
	"raster_base64",
	"thumbnail",
)

var contentKeys = setOf(
	"text",
	"units",
	"ocr_text",
	"selected_text",
	"image",
	"image_bytes",
	"raster",
	"raster_base64",
	"thumbnail",
)

var allowedModePayloadKeys = map[string]map[string]struct{}{
	"dom": setOf("units", "capture_method_version", "viewport", "gaps", "uncertainty"),
	"redacted_visual": setOf(
		"units",
		"redaction_policy_version",
		"capture_method_version",
		"viewport",
		"gaps",
		"uncertainty",
	),
	"fingerprint_only": setOf(
		"visual_fingerprint",
		"fingerprint_method_version",
		"viewport",
		"semantic_content_unavailable",
		"reason",
		"change_distance",
	),
	"metadata_only": setOf("reason", "capture_method_version", "viewport", "gaps", "uncertainty"),
	"blocked":       setOf("reason", "rule_id"),
	"failed":        setOf("reason", "error_code", "capture_method_version", "gap", "uncertainty"),
}

func DecodeAndValidate(raw json.RawMessage) (ValidatedEvent, error) {
	var zero ValidatedEvent
	if len(raw) == 0 || len(raw) > MaxEventBytes {
		return zero, validationError("EVENT_TOO_LARGE_OR_EMPTY")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, validationError("INVALID_JSON")
	}

	var shapeErrors []string
	for field := range requiredTopLevelFields {
		value, ok := fields[field]
		if !ok {
			shapeErrors = append(shapeErrors, "MISSING_FIELD:"+field)
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			shapeErrors = append(shapeErrors, "NULL_REQUIRED_FIELD:"+field)
		}
	}
	for field := range fields {
		if _, ok := allowedTopLevelFields[field]; !ok {
			shapeErrors = append(shapeErrors, "UNKNOWN_TOP_LEVEL_FIELD:"+field)
		}
	}
	for _, optional := range []string{"browser_instance_id", "view_id", "capture_mode", "source"} {
		if value, ok := fields[optional]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			shapeErrors = append(shapeErrors, "NULL_OPTIONAL_FIELD:"+optional)
		}
	}
	if len(shapeErrors) > 0 {
		sort.Strings(shapeErrors)
		return zero, &ValidationError{Codes: deduplicate(shapeErrors)}
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return zero, validationError("INVALID_SCHEMA")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, validationError("TRAILING_JSON")
	}

	if err := Validate(event); err != nil {
		return zero, err
	}
	return ValidatedEvent{event: event}, nil
}

func Validate(event Event) error {
	var codes []string

	if event.SchemaVersion != SchemaVersion {
		codes = append(codes, "INCOMPATIBLE_SCHEMA")
	}
	if event.EventID == "" || len(event.EventID) > 128 {
		codes = append(codes, "INVALID_EVENT_ID")
	}
	if event.SessionID == "" || len(event.SessionID) > 128 {
		codes = append(codes, "INVALID_SESSION_ID")
	}
	if _, ok := eventTypes[event.EventType]; !ok {
		codes = append(codes, "UNKNOWN_EVENT_TYPE")
	}
	if event.WallTime == "" {
		codes = append(codes, "INVALID_WALL_TIME")
	} else if _, err := time.Parse(time.RFC3339Nano, event.WallTime); err != nil {
		codes = append(codes, "INVALID_WALL_TIME")
	}
	if math.IsNaN(event.MonotonicMS) || math.IsInf(event.MonotonicMS, 0) || event.MonotonicMS < 0 {
		codes = append(codes, "INVALID_MONOTONIC_TIME")
	}
	if event.BrowserInstanceID != "" && len(event.BrowserInstanceID) > 128 {
		codes = append(codes, "INVALID_BROWSER_INSTANCE_ID")
	}
	if event.ViewID != "" && len(event.ViewID) > 128 {
		codes = append(codes, "INVALID_VIEW_ID")
	}
	if event.Payload == nil {
		codes = append(codes, "INVALID_PAYLOAD")
	}

	if event.CaptureMode != "" {
		if _, ok := captureModes[event.CaptureMode]; !ok {
			codes = append(codes, "UNKNOWN_CAPTURE_MODE")
		}
	}
	if event.EventType == "content_observation" && event.CaptureMode == "" {
		codes = append(codes, "CAPTURE_MODE_REQUIRED")
	}

	if event.Source != nil {
		if (event.Source.URL == "" && event.Source.CanonicalURL == "") || len(event.Source.URL) > 8192 || len(event.Source.CanonicalURL) > 8192 || len(event.Source.Title) > 2048 {
			codes = append(codes, "INVALID_SOURCE")
		}
	}

	nestedKeys := collectKeys(event.Payload)
	for key := range nestedKeys {
		if _, forbidden := forbiddenPayloadKeys[key]; forbidden {
			codes = append(codes, "FORBIDDEN_PAYLOAD_FIELD:"+key)
		}
	}

	if allowed, ok := allowedModePayloadKeys[event.CaptureMode]; ok {
		for key := range event.Payload {
			if _, permitted := allowed[key]; !permitted {
				codes = append(codes, "UNEXPECTED_MODE_PAYLOAD_FIELD:"+key)
			}
		}
	}

	if event.CaptureMode == "blocked" {
		if event.Source != nil {
			codes = append(codes, "BLOCKED_SOURCE_FORBIDDEN")
		}
		if intersects(nestedKeys, contentKeys) {
			codes = append(codes, "BLOCKED_CONTENT_FORBIDDEN")
		}
	}
	if event.CaptureMode == "fingerprint_only" || event.CaptureMode == "metadata_only" {
		if intersects(nestedKeys, contentKeys) {
			codes = append(codes, strings.ToUpper(event.CaptureMode)+"_CONTENT_FORBIDDEN")
		}
	}
	if event.CaptureMode == "redacted_visual" {
		value, ok := event.Payload["redaction_policy_version"].(string)
		if !ok || value == "" {
			codes = append(codes, "REDACTION_POLICY_REQUIRED")
		}
	}

	if len(codes) > 0 {
		sort.Strings(codes)
		return &ValidationError{Codes: deduplicate(codes)}
	}
	return nil
}

func collectKeys(value any) map[string]struct{} {
	out := make(map[string]struct{})
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			for key, nested := range typed {
				out[key] = struct{}{}
				walk(nested)
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
	return out
}

func intersects(left, right map[string]struct{}) bool {
	for key := range left {
		if _, ok := right[key]; ok {
			return true
		}
	}
	return false
}

func setOf(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validationError(code string) error {
	return &ValidationError{Codes: []string{code}}
}

func deduplicate(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
