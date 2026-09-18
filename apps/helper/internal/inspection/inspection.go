package inspection

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/store"
)

var (
	ErrKeyProviderRequired = errors.New("inspection key provider required")
	ErrInspectionNotReady  = errors.New("read-only inspection not ready")
)

type Options struct {
	Paths       *platformpath.Paths
	KeyProvider store.KeyProvider
}

type Runtime struct {
	Paths  platformpath.Paths
	Reader *store.ReadOnlyFileStore
}

// Open creates the CLI/read-only observation path. It never opens helper
// session authority and never calls platformpath.Ensure, so inspection does not
// create/repair profile state merely by being invoked.
//
// It also does not call KeyProvider.CurrentKey. Existing encrypted records name
// the historical key IDs they need, and Decode resolves those through KeyByID.
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
		return nil, fmt.Errorf("%w: resolve platform paths: %v", ErrInspectionNotReady, err)
	}

	codec, err := store.NewAESGCMCodec(options.KeyProvider)
	if err != nil {
		return nil, fmt.Errorf("%w: create encrypted record codec: %v", ErrInspectionNotReady, err)
	}
	reader, err := store.NewReadOnlyFileStore(paths.Observations, codec)
	if err != nil {
		return nil, fmt.Errorf("%w: open observations read-only: %v", ErrInspectionNotReady, err)
	}

	return &Runtime{Paths: paths, Reader: reader}, nil
}

func resolvePaths(configured *platformpath.Paths) (platformpath.Paths, error) {
	if configured != nil {
		return *configured, nil
	}
	return platformpath.Current()
}
