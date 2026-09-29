package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/app"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/profilelock"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/session"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

var (
	ErrKeyProviderRequired = errors.New("production key provider required")
	ErrBootstrapNotReady   = errors.New("production helper bootstrap not ready")
)

// Options contains the replaceable platform-specific inputs needed to open the
// authoritative helper runtime. KeyProvider must be backed by the required OS
// secret store in production; tests may inject an in-memory provider.
//
// Paths may be nil to use the current platform-local CurioTrace paths.
type Options struct {
	Paths       *platformpath.Paths
	KeyProvider store.KeyProvider
}

// Runtime is the authoritative native-helper composition root.
//
// It is intentionally not a general CLI/session-reader bootstrap. Opening the
// session Authority has restart semantics: a durable RECORDING/PAUSED snapshot
// is converted to INTERRUPTED. A separate CLI process must therefore never call
// Open merely to inspect a running profile.
type Runtime struct {
	Paths     platformpath.Paths
	Store     *store.FileStore
	Authority *session.Authority
	Handler   *app.Handler
	Lock      *profilelock.Lock
}

func Open(ctx context.Context, options Options) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.KeyProvider == nil {
		return nil, ErrKeyProviderRequired
	}

	paths, err := resolvePaths(options.Paths)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve platform paths: %v", ErrBootstrapNotReady, err)
	}
	if err := platformpath.Ensure(paths); err != nil {
		return nil, fmt.Errorf("%w: prepare platform paths: %v", ErrBootstrapNotReady, err)
	}

	lock, err := profilelock.Acquire(paths.Authority)
	if err != nil {
		return nil, fmt.Errorf("%w: acquire profile ownership: %w", ErrBootstrapNotReady, err)
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()

	codec, err := store.NewAESGCMCodec(options.KeyProvider)
	if err != nil {
		return nil, fmt.Errorf("%w: create encrypted record codec: %v", ErrBootstrapNotReady, err)
	}
	durableStore, err := store.NewFileStore(paths.Observations, codec)
	if err != nil {
		return nil, fmt.Errorf("%w: open durable observation store: %v", ErrBootstrapNotReady, err)
	}
	if err := durableStore.Ready(ctx); err != nil {
		return nil, fmt.Errorf("%w: durable observation store unavailable: %v", ErrBootstrapNotReady, err)
	}

	repository, err := session.NewFileSnapshotRepository(paths.Authority)
	if err != nil {
		return nil, fmt.Errorf("%w: open durable authority repository: %v", ErrBootstrapNotReady, err)
	}
	authority, err := session.OpenAuthority(ctx, repository)
	if err != nil {
		return nil, fmt.Errorf("%w: open durable session authority: %v", ErrBootstrapNotReady, err)
	}

	runtime := &Runtime{
		Paths:     paths,
		Store:     durableStore,
		Authority: authority,
		Handler:   app.NewHandlerWithAuthorityAndStore(authority, durableStore),
		Lock:      lock,
	}
	keepLock = true
	return runtime, nil
}

func (r *Runtime) Ready(ctx context.Context) error {
	if r == nil || r.Store == nil || r.Authority == nil || r.Handler == nil || r.Lock == nil || !r.Lock.Held() {
		return ErrBootstrapNotReady
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.Store.Ready(ctx); err != nil {
		return fmt.Errorf("%w: durable observation store unavailable: %v", ErrBootstrapNotReady, err)
	}
	if !r.Authority.IsDurable() {
		return fmt.Errorf("%w: session authority is not durable", ErrBootstrapNotReady)
	}
	return nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var authorityErr error
	if r.Authority != nil {
		snapshot := r.Authority.Snapshot()
		if snapshot.State == session.Recording || snapshot.State == session.Paused {
			_, authorityErr = r.Authority.Interrupt()
		}
	}
	var lockErr error
	if r.Lock != nil {
		lockErr = r.Lock.Close()
	}
	return errors.Join(authorityErr, lockErr)
}

func resolvePaths(configured *platformpath.Paths) (platformpath.Paths, error) {
	if configured != nil {
		return *configured, nil
	}
	return platformpath.Current()
}
