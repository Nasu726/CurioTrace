package app

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
)

const helperVersion = "0.0.0-dev"

type Handler struct {
	authority *session.Authority
}

func NewHandler() *Handler {
	return &Handler{authority: session.NewAuthority()}
}

func (h *Handler) Handle(message protocol.Envelope) protocol.Envelope {
	if major(message.ProtocolVersion) != major(protocol.Version) {
		return ack(message, false, "INCOMPATIBLE_PROTOCOL", nil)
	}

	switch message.Kind {
	case "hello":
		snapshot := h.authority.Snapshot()
		return reply(message, "hello.ack", map[string]any{
			"helper_version": helperVersion,
			"compatible": true,
			"required_capabilities": []string{},
			"helper_capabilities": []string{
				"recording_epoch_v1",
				"fail_closed_disconnect_v1",
			},
			"session_state": snapshot.State,
			"session_id": nullableString(snapshot.SessionID),
			"recording_epoch": snapshot.RecordingEpoch,
		})

	case "session.start":
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
		if !h.authority.AcceptsObservation(message.SessionID, message.RecordingEpoch) {
			return ack(message, false, "STALE_OR_UNAUTHORIZED_AUTHORITY", nil)
		}
		// Deliberately fail closed until the production observation validator/store
		// are implemented. The reference Python harness remains the conformance
		// oracle for observation semantics in the meantime.
		return ack(message, false, "OBSERVATION_PIPELINE_NOT_IMPLEMENTED", nil)

	default:
		return ack(message, false, "UNKNOWN_MESSAGE_KIND", nil)
	}
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
		"state": snapshot.State,
		"session_id": nullableString(snapshot.SessionID),
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
		MessageID: request.MessageID,
		Kind: kind,
		Payload: raw,
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
