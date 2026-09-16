package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	Version                 = "1.0"
	MaxInboundMessageBytes  = 64 * 1024 * 1024
	MaxOutboundMessageBytes = 1 * 1024 * 1024
)

var (
	ErrMessageTooLarge = errors.New("native message too large")
	ErrTruncated       = errors.New("truncated native message")
	ErrInvalidJSON     = errors.New("invalid native message JSON")
)

type Envelope struct {
	ProtocolVersion string          `json:"protocol_version"`
	MessageID       string          `json:"message_id,omitempty"`
	Kind            string          `json:"kind"`
	SessionID       string          `json:"session_id,omitempty"`
	RecordingEpoch  uint64          `json:"recording_epoch,omitempty"`
	BrowserInstance string          `json:"browser_instance_id,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

func DecodePayload[T any](message Envelope) (T, error) {
	var value T
	if len(message.Payload) == 0 {
		return value, nil
	}
	if err := json.Unmarshal(message.Payload, &value); err != nil {
		return value, fmt.Errorf("decode payload: %w", err)
	}
	return value, nil
}

func EncodePayload(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`{}`), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return data, nil
}

func Read(r io.Reader) (Envelope, error) {
	var message Envelope
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return message, io.EOF
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return message, ErrTruncated
		}
		return message, err
	}

	size := binary.NativeEndian.Uint32(prefix[:])
	if size > MaxInboundMessageBytes {
		return message, ErrMessageTooLarge
	}

	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return message, ErrTruncated
		}
		return message, err
	}

	if err := json.Unmarshal(payload, &message); err != nil {
		return message, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return message, nil
}

func Write(w io.Writer, message Envelope) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode native message: %w", err)
	}
	if len(payload) > MaxOutboundMessageBytes {
		return ErrMessageTooLarge
	}

	var prefix [4]byte
	binary.NativeEndian.PutUint32(prefix[:], uint32(len(payload)))
	if _, err := w.Write(prefix[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
