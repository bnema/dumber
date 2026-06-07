package port

import "errors"

// ErrFaviconMiss is returned when a favicon cannot be found or served from allowed sources.
var ErrFaviconMiss = errors.New("favicon miss")

// ErrFaviconInvalidInput is returned when favicon storage receives invalid input.
var ErrFaviconInvalidInput = errors.New("invalid favicon input")
