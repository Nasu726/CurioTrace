package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

var errTestKeyNotFound = errors.New("test key not found")

type testKeyProvider struct {
	current string
	keys    map[string][]byte
}

func (p *testKeyProvider) CurrentKey(_ context.Context) (KeyMaterial, error) {
	return p.KeyByID(context.Background(), p.current)
}

func (p *testKeyProvider) KeyByID(_ context.Context, keyID string) (KeyMaterial, error) {
	key, ok := p.keys[keyID]
	if !ok {
		return KeyMaterial{}, errTestKeyNotFound
	}
	return KeyMaterial{ID: keyID, Key: bytes.Clone(key)}, nil
}

func TestAESGCMCodecRoundTripDoesNotExposePlaintext(t *testing.T) {
	provider := testKeys("key-a")
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	event := testValidatedEvent(t, "ses_crypto", "evt_1", "sensitive visible text")

	record, err := codec.Encode(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(record, []byte("sensitive visible text")) || bytes.Contains(record, []byte("ses_crypto")) {
		t.Fatalf("encrypted record exposed plaintext: %q", record)
	}

	decoded, err := codec.Decode(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.EventID() != event.EventID() || decoded.SessionID() != event.SessionID() {
		t.Fatalf("unexpected decrypted event: id=%q session=%q", decoded.EventID(), decoded.SessionID())
	}
}

func TestAESGCMCodecUsesFreshNonce(t *testing.T) {
	codec, err := NewAESGCMCodec(testKeys("key-a"))
	if err != nil {
		t.Fatal(err)
	}
	event := testValidatedEvent(t, "ses_nonce", "evt_1", "same plaintext")
	first, err := codec.Encode(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codec.Encode(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("identical plaintext produced identical AES-GCM records")
	}
}

func TestAESGCMCodecFailsClosedOnTamper(t *testing.T) {
	codec, err := NewAESGCMCodec(testKeys("key-a"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := codec.Encode(context.Background(), testValidatedEvent(t, "ses_tamper", "evt_1", "secret"))
	if err != nil {
		t.Fatal(err)
	}
	record[len(record)-1] ^= 0xff

	_, err = codec.Decode(context.Background(), record)
	if !errors.Is(err, ErrRecordAuthentication) {
		t.Fatalf("tampered record did not fail authentication: %v", err)
	}
}

func TestAESGCMCodecFailsClosedWithWrongKey(t *testing.T) {
	writer, err := NewAESGCMCodec(testKeys("key-a"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := writer.Encode(context.Background(), testValidatedEvent(t, "ses_wrong", "evt_1", "secret"))
	if err != nil {
		t.Fatal(err)
	}

	wrong := &testKeyProvider{
		current: "key-a",
		keys: map[string][]byte{
			"key-a": bytes.Repeat([]byte{0x77}, AES256KeyBytes),
		},
	}
	reader, err := NewAESGCMCodec(wrong)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.Decode(context.Background(), record)
	if !errors.Is(err, ErrRecordAuthentication) {
		t.Fatalf("wrong key did not fail authentication: %v", err)
	}
}

func TestAESGCMCodecPropagatesMissingHistoricalKey(t *testing.T) {
	writerProvider := testKeys("key-old")
	writer, err := NewAESGCMCodec(writerProvider)
	if err != nil {
		t.Fatal(err)
	}
	record, err := writer.Encode(context.Background(), testValidatedEvent(t, "ses_missing", "evt_1", "secret"))
	if err != nil {
		t.Fatal(err)
	}

	reader, err := NewAESGCMCodec(testKeys("key-new"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.Decode(context.Background(), record)
	if !errors.Is(err, errTestKeyNotFound) {
		t.Fatalf("missing historical key was not surfaced: %v", err)
	}
}

func TestAESGCMCodecSupportsKeyRotation(t *testing.T) {
	provider := &testKeyProvider{
		current: "key-old",
		keys: map[string][]byte{
			"key-old": bytes.Repeat([]byte{0x11}, AES256KeyBytes),
			"key-new": bytes.Repeat([]byte{0x22}, AES256KeyBytes),
		},
	}
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	oldEvent := testValidatedEvent(t, "ses_rotate", "evt_old", "old")
	oldRecord, err := codec.Encode(context.Background(), oldEvent)
	if err != nil {
		t.Fatal(err)
	}

	provider.current = "key-new"
	newEvent := testValidatedEvent(t, "ses_rotate", "evt_new", "new")
	newRecord, err := codec.Encode(context.Background(), newEvent)
	if err != nil {
		t.Fatal(err)
	}

	decodedOld, err := codec.Decode(context.Background(), oldRecord)
	if err != nil {
		t.Fatal(err)
	}
	decodedNew, err := codec.Decode(context.Background(), newRecord)
	if err != nil {
		t.Fatal(err)
	}
	if decodedOld.EventID() != "evt_old" || decodedNew.EventID() != "evt_new" {
		t.Fatalf("rotation changed durable events: old=%q new=%q", decodedOld.EventID(), decodedNew.EventID())
	}
}

func TestAESGCMCodecRejectsInvalidKeyMaterial(t *testing.T) {
	provider := &testKeyProvider{
		current: "short",
		keys: map[string][]byte{
			"short": []byte("not-32-bytes"),
		},
	}
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	_, err = codec.Encode(context.Background(), testValidatedEvent(t, "ses_badkey", "evt_1", "secret"))
	if !errors.Is(err, ErrInvalidKeyMaterial) {
		t.Fatalf("invalid AES-256 key was accepted: %v", err)
	}
}

func TestEncryptedFileStorePersistsOnlyCiphertextAcrossReopen(t *testing.T) {
	root := t.TempDir()
	provider := testKeys("key-a")
	codec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewFileStore(root, codec)
	if err != nil {
		t.Fatal(err)
	}
	event := testValidatedEvent(t, "ses_file_crypto", "evt_1", "durable secret canary")
	if err := first.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	stored, err := os.ReadFile(first.sessionPath("ses_file_crypto"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("durable secret canary")) || bytes.Contains(stored, []byte("ses_file_crypto")) {
		t.Fatalf("durable log exposed plaintext event content: %q", stored)
	}

	secondCodec, err := NewAESGCMCodec(provider)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileStore(root, secondCodec)
	if err != nil {
		t.Fatal(err)
	}
	events, err := second.ListSession(context.Background(), "ses_file_crypto")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID() != "evt_1" {
		t.Fatalf("encrypted durable session did not survive reopen: %#v", events)
	}
}

func testKeys(current string) *testKeyProvider {
	return &testKeyProvider{
		current: current,
		keys: map[string][]byte{
			current: bytes.Repeat([]byte{0x41}, AES256KeyBytes),
		},
	}
}
