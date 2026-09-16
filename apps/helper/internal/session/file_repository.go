package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	snapshotSchemaVersion = 1
	maxSnapshotBytes      = 4 * 1024
	maxSnapshotLogBytes   = 4 * 1024 * 1024
)

var (
	snapshotLogMagic     = [8]byte{'C', 'T', 'S', 'T', 'A', 'T', 'E', '1'}
	snapshotCRCTable     = crc32.MakeTable(crc32.Castagnoli)
	ErrCorruptStateLog   = errors.New("corrupt durable session state log")
	ErrStateLogTooLarge  = errors.New("durable session state log too large")
	ErrInvalidStateStore = errors.New("invalid durable session state store configuration")
)

type persistedSnapshot struct {
	Version        int    `json:"version"`
	State          State  `json:"state"`
	SessionID      string `json:"session_id"`
	RecordingEpoch uint64 `json:"recording_epoch"`
}

// FileSnapshotRepository is a small append-only control-state journal. It
// persists only state/session-id/epoch metadata; browsing observations belong
// to the encrypted observation store instead.
type FileSnapshotRepository struct {
	root string
	path string
	mu   sync.Mutex
}

func NewFileSnapshotRepository(root string) (*FileSnapshotRepository, error) {
	if root == "" {
		return nil, ErrInvalidStateStore
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create session state directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("protect session state directory: %w", err)
	}
	return &FileSnapshotRepository{
		root: root,
		path: filepath.Join(root, "authority.state"),
	}, nil
}

func (r *FileSnapshotRepository) Load(ctx context.Context) (Snapshot, bool, error) {
	var zero Snapshot
	if err := contextErr(ctx); err != nil {
		return zero, false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	file, err := os.OpenFile(r.path, os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, fmt.Errorf("open durable session state: %w", err)
	}
	defer file.Close()
	if err := os.Chmod(r.path, 0o600); err != nil {
		return zero, false, fmt.Errorf("protect durable session state: %w", err)
	}

	return r.scanLocked(ctx, file, true)
}

func (r *FileSnapshotRepository) Save(ctx context.Context, snapshot Snapshot) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(persistedSnapshot{
		Version:        snapshotSchemaVersion,
		State:          snapshot.State,
		SessionID:      snapshot.SessionID,
		RecordingEpoch: snapshot.RecordingEpoch,
	})
	if err != nil {
		return fmt.Errorf("marshal durable session state: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxSnapshotBytes {
		return ErrCorruptStateLog
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	file, created, err := r.openLocked()
	if err != nil {
		return err
	}
	defer file.Close()

	if !created {
		if _, _, err := r.scanLocked(ctx, file, true); err != nil {
			return err
		}
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek durable session state: %w", err)
	}

	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[4:8], crc32.Checksum(payload, snapshotCRCTable))
	copy(frame[8:], payload)
	if err := writeAllState(file, frame); err != nil {
		return fmt.Errorf("append durable session state: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync durable session state: %w", err)
	}
	return nil
}

func (r *FileSnapshotRepository) openLocked() (*os.File, bool, error) {
	file, err := os.OpenFile(r.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open durable session state: %w", err)
	}
	if err := os.Chmod(r.path, 0o600); err != nil {
		file.Close()
		return nil, false, fmt.Errorf("protect durable session state: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, false, fmt.Errorf("stat durable session state: %w", err)
	}
	if info.Size() > maxSnapshotLogBytes {
		file.Close()
		return nil, false, ErrStateLogTooLarge
	}
	if info.Size() != 0 {
		return file, false, nil
	}
	if err := writeAllState(file, snapshotLogMagic[:]); err != nil {
		file.Close()
		return nil, false, fmt.Errorf("initialize durable session state: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, false, fmt.Errorf("sync durable session state header: %w", err)
	}
	if err := syncStateDirectory(r.root); err != nil {
		file.Close()
		return nil, false, err
	}
	return file, true, nil
}

func (r *FileSnapshotRepository) scanLocked(ctx context.Context, file *os.File, repairTail bool) (Snapshot, bool, error) {
	var zero Snapshot
	if err := contextErr(ctx); err != nil {
		return zero, false, err
	}
	info, err := file.Stat()
	if err != nil {
		return zero, false, fmt.Errorf("stat durable session state: %w", err)
	}
	if info.Size() > maxSnapshotLogBytes {
		return zero, false, ErrStateLogTooLarge
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return zero, false, fmt.Errorf("seek durable session state header: %w", err)
	}

	header := make([]byte, len(snapshotLogMagic))
	if _, err := io.ReadFull(file, header); err != nil {
		return zero, false, ErrCorruptStateLog
	}
	if !bytes.Equal(header, snapshotLogMagic[:]) {
		return zero, false, ErrCorruptStateLog
	}

	offset := int64(len(snapshotLogMagic))
	var latest Snapshot
	found := false
	for {
		if err := contextErr(ctx); err != nil {
			return zero, false, err
		}
		frameHeader := make([]byte, 8)
		_, err := io.ReadFull(file, frameHeader)
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			if err := repairSnapshotTail(file, offset, repairTail); err != nil {
				return zero, false, err
			}
			break
		}
		if err != nil {
			return zero, false, fmt.Errorf("read durable session state frame: %w", err)
		}

		length := int(binary.BigEndian.Uint32(frameHeader[0:4]))
		checksum := binary.BigEndian.Uint32(frameHeader[4:8])
		if length <= 0 || length > maxSnapshotBytes {
			return zero, false, ErrCorruptStateLog
		}
		payload := make([]byte, length)
		_, err = io.ReadFull(file, payload)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			if err := repairSnapshotTail(file, offset, repairTail); err != nil {
				return zero, false, err
			}
			break
		}
		if err != nil {
			return zero, false, fmt.Errorf("read durable session state payload: %w", err)
		}
		if crc32.Checksum(payload, snapshotCRCTable) != checksum {
			return zero, false, ErrCorruptStateLog
		}
		snapshot, err := decodePersistedSnapshot(payload)
		if err != nil {
			return zero, false, err
		}
		latest = snapshot
		found = true
		offset += int64(8 + length)
	}
	if !found {
		return zero, false, ErrCorruptStateLog
	}
	return latest, true, nil
}

func decodePersistedSnapshot(payload []byte) (Snapshot, error) {
	var record persistedSnapshot
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Snapshot{}, fmt.Errorf("%w: decode snapshot: %v", ErrCorruptStateLog, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Snapshot{}, ErrCorruptStateLog
	}
	if record.Version != snapshotSchemaVersion {
		return Snapshot{}, ErrCorruptStateLog
	}
	snapshot := Snapshot{
		State:          record.State,
		SessionID:      record.SessionID,
		RecordingEpoch: record.RecordingEpoch,
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrCorruptStateLog, err)
	}
	return snapshot, nil
}

func repairSnapshotTail(file *os.File, offset int64, repair bool) error {
	if !repair {
		return ErrCorruptStateLog
	}
	if err := file.Truncate(offset); err != nil {
		return fmt.Errorf("repair partial durable session state tail: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync repaired durable session state: %w", err)
	}
	return nil
}

func writeAllState(writer io.Writer, data []byte) error {
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

func syncStateDirectory(root string) error {
	// Directory fsync is meaningful on Unix. Go/Windows does not provide a
	// portable directory handle sync contract; the state file itself is still
	// synchronously flushed before Start/Resume acknowledgement.
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("open session state directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync session state directory: %w", err)
	}
	return nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
