package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
)

const (
	MaxStoredRecordBytes = 512 * 1024
	maxCodecIDBytes      = 128
)

var (
	logMagic           = [8]byte{'C', 'T', 'L', 'O', 'G', '1', '\r', '\n'}
	crcTable           = crc32.MakeTable(crc32.Castagnoli)
	ErrInvalidStore    = errors.New("invalid durable store configuration")
	ErrCodecMismatch   = errors.New("durable log codec mismatch")
	ErrCorruptRecord   = errors.New("corrupt durable record")
	ErrRecordTooLarge  = errors.New("durable record too large")
	ErrSessionMismatch = errors.New("durable record belongs to another session")
	ErrEventIDConflict = errors.New("event id reused with different content")
)

type FileStore struct {
	root  string
	codec RecordCodec

	mu      sync.Mutex
	indexes map[string]map[string][32]byte
}

func NewFileStore(root string, codec RecordCodec) (*FileStore, error) {
	if root == "" || codec == nil {
		return nil, ErrInvalidStore
	}
	codecID := codec.ID()
	if codecID == "" || len(codecID) > maxCodecIDBytes {
		return nil, ErrInvalidStore
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create durable store directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("protect durable store directory: %w", err)
	}
	return &FileStore{
		root:    root,
		codec:   codec,
		indexes: make(map[string]map[string][32]byte),
	}, nil
}

func (s *FileStore) Append(_ context.Context, event observation.ValidatedEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := event.SessionID()
	index, err := s.ensureIndexLocked(sessionID)
	if err != nil {
		return err
	}

	digest, err := validatedDigest(event)
	if err != nil {
		return fmt.Errorf("digest durable event: %w", err)
	}
	if existing, ok := index[event.EventID()]; ok {
		if existing == digest {
			return nil
		}
		return ErrEventIDConflict
	}

	record, err := s.codec.Encode(event)
	if err != nil {
		return fmt.Errorf("encode durable event: %w", err)
	}
	if len(record) == 0 || len(record) > MaxStoredRecordBytes {
		return ErrRecordTooLarge
	}

	file, err := s.openLog(sessionID, true)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek durable log: %w", err)
	}

	frame := make([]byte, 8+len(record))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(record)))
	binary.BigEndian.PutUint32(frame[4:8], crc32.Checksum(record, crcTable))
	copy(frame[8:], record)
	if err := writeAll(file, frame); err != nil {
		return fmt.Errorf("append durable record: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync durable record: %w", err)
	}

	index[event.EventID()] = digest
	return nil
}

func (s *FileStore) ListSession(_ context.Context, sessionID string) ([]observation.ValidatedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.openLog(sessionID, false)
	if errors.Is(err, os.ErrNotExist) {
		return []observation.ValidatedEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events, index, err := s.scanAndRepair(file, sessionID)
	if err != nil {
		return nil, err
	}
	s.indexes[sessionID] = index
	return events, nil
}

func (s *FileStore) DeleteSession(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(sessionID)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete durable session log: %w", err)
	}
	delete(s.indexes, sessionID)
	return nil
}

func (s *FileStore) ensureIndexLocked(sessionID string) (map[string][32]byte, error) {
	if index, ok := s.indexes[sessionID]; ok {
		return index, nil
	}

	file, err := s.openLog(sessionID, true)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_, index, err := s.scanAndRepair(file, sessionID)
	if err != nil {
		return nil, err
	}
	s.indexes[sessionID] = index
	return index, nil
}

func (s *FileStore) openLog(sessionID string, create bool) (*os.File, error) {
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	file, err := os.OpenFile(s.sessionPath(sessionID), flags, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("protect durable session log: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.Size() == 0 {
		if !create {
			file.Close()
			return nil, fmt.Errorf("%w: empty durable log", ErrCorruptRecord)
		}
		if err := s.writeHeader(file); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (s *FileStore) writeHeader(file *os.File) error {
	codecID := []byte(s.codec.ID())
	header := make([]byte, len(logMagic)+2+len(codecID))
	copy(header, logMagic[:])
	binary.BigEndian.PutUint16(header[len(logMagic):len(logMagic)+2], uint16(len(codecID)))
	copy(header[len(logMagic)+2:], codecID)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := writeAll(file, header); err != nil {
		return err
	}
	return file.Sync()
}

func (s *FileStore) scanAndRepair(file *os.File, expectedSessionID string) ([]observation.ValidatedEvent, map[string][32]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	headerEnd, err := s.readHeader(file)
	if err != nil {
		return nil, nil, err
	}

	events := make([]observation.ValidatedEvent, 0)
	index := make(map[string][32]byte)
	lastGood := headerEnd
	if _, err := file.Seek(lastGood, io.SeekStart); err != nil {
		return nil, nil, err
	}

	for {
		var frameHeader [8]byte
		n, err := io.ReadFull(file, frameHeader[:])
		if errors.Is(err, io.EOF) && n == 0 {
			return events, index, nil
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			if err := repairTail(file, lastGood); err != nil {
				return nil, nil, err
			}
			return events, index, nil
		}
		if err != nil {
			return nil, nil, err
		}

		recordLen := binary.BigEndian.Uint32(frameHeader[0:4])
		wantCRC := binary.BigEndian.Uint32(frameHeader[4:8])
		if recordLen == 0 || recordLen > MaxStoredRecordBytes {
			return nil, nil, ErrRecordTooLarge
		}
		record := make([]byte, int(recordLen))
		if _, err := io.ReadFull(file, record); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				if err := repairTail(file, lastGood); err != nil {
					return nil, nil, err
				}
				return events, index, nil
			}
			return nil, nil, err
		}
		if crc32.Checksum(record, crcTable) != wantCRC {
			return nil, nil, ErrCorruptRecord
		}

		event, err := s.codec.Decode(record)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: decode record: %v", ErrCorruptRecord, err)
		}
		if event.SessionID() != expectedSessionID {
			return nil, nil, ErrSessionMismatch
		}
		digest, err := validatedDigest(event)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: canonicalize record: %v", ErrCorruptRecord, err)
		}
		if existing, ok := index[event.EventID()]; ok {
			if existing != digest {
				return nil, nil, ErrEventIDConflict
			}
		} else {
			index[event.EventID()] = digest
			events = append(events, event)
		}

		position, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, nil, err
		}
		lastGood = position
	}
}

func (s *FileStore) readHeader(file *os.File) (int64, error) {
	var magic [8]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return 0, fmt.Errorf("%w: truncated header", ErrCorruptRecord)
	}
	if magic != logMagic {
		return 0, fmt.Errorf("%w: invalid magic", ErrCorruptRecord)
	}

	var length [2]byte
	if _, err := io.ReadFull(file, length[:]); err != nil {
		return 0, fmt.Errorf("%w: truncated codec id", ErrCorruptRecord)
	}
	codecLen := binary.BigEndian.Uint16(length[:])
	if codecLen == 0 || codecLen > maxCodecIDBytes {
		return 0, ErrCodecMismatch
	}
	codecID := make([]byte, int(codecLen))
	if _, err := io.ReadFull(file, codecID); err != nil {
		return 0, fmt.Errorf("%w: truncated codec id", ErrCorruptRecord)
	}
	if !bytes.Equal(codecID, []byte(s.codec.ID())) {
		return 0, ErrCodecMismatch
	}
	return int64(len(logMagic) + 2 + len(codecID)), nil
}

func (s *FileStore) sessionPath(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return filepath.Join(s.root, hex.EncodeToString(hash[:])+".ctlog")
}

func validatedDigest(event observation.ValidatedEvent) ([32]byte, error) {
	var zero [32]byte
	canonical, err := json.Marshal(event)
	if err != nil {
		return zero, err
	}
	return sha256.Sum256(canonical), nil
}

func repairTail(file *os.File, lastGood int64) error {
	if err := file.Truncate(lastGood); err != nil {
		return fmt.Errorf("repair partial durable tail: %w", err)
	}
	if _, err := file.Seek(lastGood, io.SeekStart); err != nil {
		return err
	}
	return file.Sync()
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
