package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestAESGCMCodecAuthenticatesKeyIDHeader(t *testing.T) {
	sharedKey := bytes.Repeat([]byte{0x55}, AES256KeyBytes)
	provider := &testKeyProvider{
		current: "key-a",
		keys: map[string][]byte{
			"key-a": sharedKey,
			"key-b": bytes.Clone(sharedKey),
		},
	}
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	record, err := codec.Encode(context.Background(), testValidatedEvent(t, "ses_aad", "evt_1", "secret"))
	if err != nil {
		t.Fatal(err)
	}

	keyIDStart := len(encryptedRecordMagic) + 2
	copy(record[keyIDStart:keyIDStart+len("key-b")], "key-b")

	_, err = codec.Decode(context.Background(), record)
	if !errors.Is(err, ErrRecordAuthentication) {
		t.Fatalf("modified key-id header was not authenticated: %v", err)
	}
}
