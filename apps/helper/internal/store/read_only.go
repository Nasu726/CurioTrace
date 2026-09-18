package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

var (
	ErrInvalidReadOnlyStore   = errors.New("invalid read-only durable store configuration")
	ErrUnsafeReadOnlyStore    = errors.New("unsafe read-only durable store path")
	ErrIncompleteDurableTail  = errors.New("incomplete durable record tail")
)

// ReadOnlyFileStore reads existing CurioTrace observation logs without creating,
// chmodding, truncating, repairing, or appending files.
//
// It deliberately does not call RecordCodec.Ready: for AES-GCM that would ask
// for CurrentKey and may provision a new key. Inspection needs only historical
// KeyByID lookups while decoding records that already exist.
type ReadOnlyFileStore struct {
	root  string
	codec RecordCodec
}

func NewReadOnlyFileStore(root string, codec RecordCodec) (*ReadOnlyFileStore, error) {
	if root == "" || codec == nil {
		return nil, ErrInvalidReadOnlyStore
	}
	codecID := codec.ID()
	if codecID == "" || len(codecID) > maxCodecIDBytes {
		return nil, ErrInvalidReadOnlyStore
	}

	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: observations root does not exist", ErrInvalidReadOnlyStore)
		}
		return nil, fmt.Errorf("inspect observations root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: observations root", ErrUnsafeReadOnlyStore)
	}

	return &ReadOnlyFileStore{root: filepath.Clean(root), codec: codec}, nil
}

func (s *ReadOnlyFileStore) ListSession(ctx context.Context, sessionID string) ([]observation.ValidatedEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path := readOnlySessionPath(s.root, sessionID)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []observation.ValidatedEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect durable session log: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: session log", ErrUnsafeReadOnlyStore)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open durable session log read-only: %w", err)
	}
	defer file.Close()

	if err := readOnlyHeader(file, s.codec.ID()); err != nil {
		return nil, err
	}

	events := make([]observation.ValidatedEvent, 0)
	index := make(map[string][32]byte)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var frameHeader [8]byte
		n, err := io.ReadFull(file, frameHeader[:])
		if errors.Is(err, io.EOF) && n == 0 {
			return events, nil
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrIncompleteDurableTail
		}
		if err != nil {
			return nil, fmt.Errorf("read durable record header: %w", err)
		}

		recordLen := binary.BigEndian.Uint32(frameHeader[0:4])
		wantCRC := binary.BigEndian.Uint32(frameHeader[4:8])
		if recordLen == 0 || recordLen > MaxStoredRecordBytes {
			return nil, ErrRecordTooLarge
		}

		record := make([]byte, int(recordLen))
		if _, err := io.ReadFull(file, record); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, ErrIncompleteDurableTail
			}
			return nil, fmt.Errorf("read durable record: %w", err)
		}
		if crc32.Checksum(record, crcTable) != wantCRC {
			return nil, ErrCorruptRecord
		}

		event, err := s.codec.Decode(ctx, record)
		if err != nil {
			return nil, fmt.Errorf("%w: decode record: %v", ErrCorruptRecord, err)
		}
		if event.SessionID() != sessionID {
			return nil, ErrSessionMismatch
		}
		digest, err := validatedDigest(event)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize record: %v", ErrCorruptRecord, err)
		}
		if existing, ok := index[event.EventID()]; ok {
			if existing != digest {
				return nil, ErrEventIDConflict
			}
			continue
		}
		index[event.EventID()] = digest
		events = append(events, event)
	}
}

func readOnlyHeader(reader io.Reader, expectedCodecID string) error {
	var magic [8]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return fmt.Errorf("%w: truncated header", ErrCorruptRecord)
	}
	if magic != logMagic {
		return fmt.Errorf("%w: invalid magic", ErrCorruptRecord)
	}

	var length [2]byte
	if _, err := io.ReadFull(reader, length[:]); err != nil {
		return fmt.Errorf("%w: truncated codec id", ErrCorruptRecord)
	}
	codecLen := binary.BigEndian.Uint16(length[:])
	if codecLen == 0 || codecLen > maxCodecIDBytes {
		return ErrCodecMismatch
	}
	codecID := make([]byte, int(codecLen))
	if _, err := io.ReadFull(reader, codecID); err != nil {
		return fmt.Errorf("%w: truncated codec id", ErrCorruptRecord)
	}
	if !bytes.Equal(codecID, []byte(expectedCodecID)) {
		return ErrCodecMismatch
	}
	return nil
}

func readOnlySessionPath(root, sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return filepath.Join(root, hex.EncodeToString(hash[:])+".ctlog")
}
