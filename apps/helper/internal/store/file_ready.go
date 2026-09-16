package store

import "context"

func (s *FileStore) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.codec.Ready(ctx)
}
