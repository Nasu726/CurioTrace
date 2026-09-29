package store

import "errors"

var (
	ErrInvalidSecretName = errors.New("invalid secure secret name")
	ErrSecretTooLarge    = errors.New("secure secret value too large")
)
