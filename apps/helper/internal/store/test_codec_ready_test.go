package store

import "context"

func (c testJSONCodec) Ready(ctx context.Context) error {
	return ctx.Err()
}
