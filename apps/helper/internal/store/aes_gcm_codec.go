package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

const (
	aesGCMCodecID = "aes256-gcm-json-v1"
	maxKeyIDBytes = 128
)

var (
	encryptedRecordMagic    = [4]byte{'C', 'T', 'E', '1'}
	ErrInvalidKeyMaterial   = errors.New("invalid encryption key material")
	ErrEncryptedRecord      = errors.New("invalid encrypted durable record")
	ErrRecordAuthentication = errors.New("durable record authentication failed")
)

// AESGCMCodec encrypts each validated observation independently with AES-256-GCM.
// The non-secret key ID is stored with each record so historical keys can remain
// readable across key rotation without changing the durable-log format.
type AESGCMCodec struct {
	keys KeyProvider
}

func NewAESGCMCodec(keys KeyProvider) (*AESGCMCodec, error) {
	if keys == nil {
		return nil, ErrInvalidKeyMaterial
	}
	return &AESGCMCodec{keys: keys}, nil
}

func (c *AESGCMCodec) ID() string { return aesGCMCodecID }

func (c *AESGCMCodec) Encode(ctx context.Context, event observation.ValidatedEvent) ([]byte, error) {
	material, err := c.keys.CurrentKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("load current durable-record key: %w", err)
	}
	defer clear(material.Key)
	if err := validateKeyMaterial(material, material.ID); err != nil {
		return nil, err
	}

	gcm, err := newAESGCM(material.Key)
	if err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal validated event: %w", err)
	}
	defer clear(plaintext)

	header := encryptedHeader(material.ID)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate durable-record nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, header)

	record := make([]byte, 0, len(header)+len(nonce)+len(sealed))
	record = append(record, header...)
	record = append(record, nonce...)
	record = append(record, sealed...)
	return record, nil
}

func (c *AESGCMCodec) Decode(ctx context.Context, record []byte) (observation.ValidatedEvent, error) {
	var zero observation.ValidatedEvent
	keyID, headerEnd, err := parseEncryptedHeader(record)
	if err != nil {
		return zero, err
	}

	material, err := c.keys.KeyByID(ctx, keyID)
	if err != nil {
		return zero, fmt.Errorf("load durable-record key %q: %w", keyID, err)
	}
	defer clear(material.Key)
	if err := validateKeyMaterial(material, keyID); err != nil {
		return zero, err
	}
	gcm, err := newAESGCM(material.Key)
	if err != nil {
		return zero, err
	}

	minimum := headerEnd + gcm.NonceSize() + gcm.Overhead()
	if len(record) < minimum {
		return zero, ErrEncryptedRecord
	}
	nonce := record[headerEnd : headerEnd+gcm.NonceSize()]
	ciphertext := record[headerEnd+gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, record[:headerEnd])
	if err != nil {
		return zero, ErrRecordAuthentication
	}
	defer clear(plaintext)

	event, err := observation.DecodeAndValidate(plaintext)
	if err != nil {
		return zero, fmt.Errorf("%w: decrypted event failed validation: %v", ErrEncryptedRecord, err)
	}
	return event, nil
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeyMaterial
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("construct AES-GCM: %w", err)
	}
	return gcm, nil
}

func validateKeyMaterial(material KeyMaterial, expectedID string) error {
	if material.ID == "" || material.ID != expectedID || len(material.ID) > maxKeyIDBytes || len(material.Key) != AES256KeyBytes {
		return ErrInvalidKeyMaterial
	}
	return nil
}

func encryptedHeader(keyID string) []byte {
	header := make([]byte, len(encryptedRecordMagic)+2+len(keyID))
	copy(header, encryptedRecordMagic[:])
	binary.BigEndian.PutUint16(header[len(encryptedRecordMagic):len(encryptedRecordMagic)+2], uint16(len(keyID)))
	copy(header[len(encryptedRecordMagic)+2:], keyID)
	return header
}

func parseEncryptedHeader(record []byte) (string, int, error) {
	minimum := len(encryptedRecordMagic) + 2
	if len(record) < minimum {
		return "", 0, ErrEncryptedRecord
	}
	for i := range encryptedRecordMagic {
		if record[i] != encryptedRecordMagic[i] {
			return "", 0, ErrEncryptedRecord
		}
	}
	keyIDLen := int(binary.BigEndian.Uint16(record[len(encryptedRecordMagic):minimum]))
	if keyIDLen == 0 || keyIDLen > maxKeyIDBytes || len(record) < minimum+keyIDLen {
		return "", 0, ErrEncryptedRecord
	}
	return string(record[minimum : minimum+keyIDLen]), minimum + keyIDLen, nil
}
