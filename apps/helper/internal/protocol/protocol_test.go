package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestNativeMessageRoundTrip(t *testing.T) {
	input := Envelope{
		ProtocolVersion: Version,
		MessageID:       "m1",
		Kind:            "hello",
		Payload:         []byte(`{"capabilities":[]}`),
	}
	var buffer bytes.Buffer
	if err := Write(&buffer, input); err != nil {
		t.Fatal(err)
	}
	output, err := Read(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if output.ProtocolVersion != input.ProtocolVersion || output.MessageID != input.MessageID || output.Kind != input.Kind {
		t.Fatalf("round trip mismatch: %#v", output)
	}
}

func TestTruncatedMessageFails(t *testing.T) {
	data := []byte{10, 0, 0, 0, '{', '}'}
	_, err := Read(bytes.NewReader(data))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestOutboundLimit(t *testing.T) {
	payload := make([]byte, 0, MaxOutboundMessageBytes+3)
	payload = append(payload, '"')
	payload = append(payload, bytes.Repeat([]byte{'x'}, MaxOutboundMessageBytes+1)...)
	payload = append(payload, '"')

	var buffer bytes.Buffer
	err := Write(&buffer, Envelope{ProtocolVersion: Version, Kind: "ack", Payload: payload})
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("expected ErrMessageTooLarge, got %v", err)
	}
}
