package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

const helperVersion = "0.0.0-dev"

type Handler struct {
	authority *session.Authority
	store     store.Store
}

func NewHandler() *Handler {
	return &Handler{authority: session.NewAuthority()}
}

func NewHandlerWithStore(durableStore store.Store) *Handler {
	return &Handler{authority: session.NewAuthority(), store: durableStore}
}

func (h *Handler) Handle(message protocol.Envelope) protocol.Envelope {
	if major(message.ProtocolVersion) != major(protocol.Version) {
		return ack(message, false, "INCOMPATIBLE_PROTOCOL", nil)
	}

	switch message.Kind {
	case "hello":
		snapshot := h.authority.Snapshot()
		return reply(message, "hello.ack", map[string]any{
			"helper_version":        helperVersion,
			"compatible":            true,
			"required_capabilities": []string{},
			"helper_capabilities": []string{
				"observation_schema_v1",
				"recording_epoch_v1",
				"fail_closed_disconnect_v1",
			},
			"session_state":   snapshot.State,
			"session_id":      nullableString(snapshot.SessionID),
			"recording_epoch": snapshot.RecordingEpoch,
		})

	case "session.start":
		if h.store == nil {
			return ack(message, false, "STORE_NOT_CONFIGURED", nil)
		}
		snapshot, err := h.authority.Start()
		if err != nil {
			return ack(message, false, "INVALID_TRANSITION", nil)
		}
		return ack(message, true, "OK", statePayload(snapshot))

	case "session.pause":
		return h.transition(message, h.authority.Pause)
	case "session.resume":
		return h.transition(message, h.authority.Resume)
	case "session.stop":
		return h.transition(message, h.authority.Stop)

	case "observation.submit":
		return h.handleObservation(message)

	default:
		return ack(message, false, "UNKNOWN_MESSAGE_KIND", nil)
	}
}

func (h *Handler) handleObservation(message protocol.Envelope) protocol.Envelope {
	snapshot := h.authority.Snapshot()
	if snapshot.State != session.Recording {
		return ack(message, false, "NOT_RECORDING", nil)
	}
	if message.SessionID != snapshot.SessionID {
		return ack(message, false, "UNKNOWN_SESSION", nil)
	}
	if message.RecordingEpoch != snapshot.RecordingEpoch {
		return ack(message, false, "STALE_EPOCH", nil)
	}

	payload, err := protocol.DecodePayload[observation.SubmitPayload](message)
	if err != nil || len(payload.Event) == 0 {
		return ack(message, false, "INVALID_SCHEMA", nil)
	}
	validated, err := observation.DecodeAndValidate(payload.Event)
	if err != nil {
		var validation *observation.ValidationError
		if errors.As(err, &validation) && len(validation.Codes) > 0 {
			return ack(message, false, validation.Codes[0], map[string]any{"errors": validation.Codes})
		}
		return ack(message, false, "INVALID_SCHEMA", nil)
	}
	if validated.EventType() == "session_state" {
		return ack(message, false, "STATE_EVENT_HELPER_AUTHORITY", nil)
	}
	if validated.SessionID() != message.SessionID {
		return ack(message, false, "UNKNOWN_SESSION", nil)
	}
	if validated.RecordingEpoch() != message.RecordingEpoch {
		return ack(message, false, "STALE_EPOCH", nil)
	}
	if h.store == nil {
		return ack(message, false, "STORE_NOT_CONFIGURED", nil)
	}
	if err := h.store.Append(context.Background(), validated); err != nil {
		return ack(message, false, "STORE_ERROR", nil)
	}
	return ack(message, true, "OK", nil)
}

type transitionFunc func(string, uint64) (session.Snapshot, error)

func (h *Handler) transition(message protocol.Envelope, transition transitionFunc) protocol.Envelope {
	snapshot, err := transition(message.SessionID, message.RecordingEpoch)
	if err != nil {
		return ack(message, false, "STALE_EPOCH_OR_INVALID_TRANSITION", nil)
	}
	return ack(message, true, "OK", statePayload(snapshot))
}

func statePayload(snapshot session.Snapshot) map[string]any {
	return map[string]any{
		"state":           snapshot.State,
		"session_id":      nullableString(snapshot.SessionID),
		"recording_epoch": snapshot.RecordingEpoch,
	}
}

func ack(request protocol.Envelope, accepted bool, reason string, extra map[string]any) protocol.Envelope {
	payload := map[string]any{"accepted": accepted, "reason": reason}
	for key, value := range extra {
		payload[key] = value
	}
	return reply(request, "ack", payload)
}

func reply(request protocol.Envelope, kind string, payload any) protocol.Envelope {
	raw, _ := json.Marshal(payload)
	return protocol.Envelope{
		ProtocolVersion: protocol.Version,
		MessageID:       request.MessageID,
		Kind:            kind,
		Payload:         raw,
	}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func major(version string) int {
	part := strings.SplitN(version, ".", 2)[0]
	value, err := strconv.Atoi(part)
	if err != nil {
		return -1
	}
	return value
}
