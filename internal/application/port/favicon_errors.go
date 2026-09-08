package port

import "errors"

// ErrFaviconMiss is returned when a favicon cannot be found or served from allowed sources.
var ErrFaviconMiss = errors.New("favicon miss")

// ErrFaviconBusy is returned to direct refresh callers when the shared
// refresh admission budget is exhausted. It is deliberately distinct from
// ErrFaviconMiss: overload is not a miss and must never read as success.
var ErrFaviconBusy = errors.New("favicon refresh overloaded")

// ErrFaviconShutdown is returned to refresh callers arriving after the
// use case began closing. Like overload, it is distinct from a miss.
var ErrFaviconShutdown = errors.New("favicon refresh shutting down")

// FaviconFetchError classifies a failed fetch while preserving
// ErrFaviconMiss matching for existing callers. StatusCode carries the
// HTTP status for HTTP failures and is zero for validation, transport,
// cancellation, or DNS failures, which must never be negatively cached.
type FaviconFetchError struct {
	StatusCode int
}

func (e *FaviconFetchError) Error() string {
	if e == nil || e.StatusCode == 0 {
		return "favicon fetch failed"
	}
	return "favicon fetch failed: unexpected HTTP status"
}

// Is preserves ErrFaviconMiss matching so callers that treat every fetch
// failure as a miss keep working; use FetchStatusCode for classification.
func (*FaviconFetchError) Is(target error) bool {
	return target == ErrFaviconMiss
}

// FetchStatusCode returns the HTTP status carried by err, if any.
func FetchStatusCode(err error) (int, bool) {
	var fetchErr *FaviconFetchError
	if errors.As(err, &fetchErr) && fetchErr != nil {
		return fetchErr.StatusCode, true
	}
	return 0, false
}

// ErrFaviconInvalidInput is returned when favicon storage receives invalid input.
var ErrFaviconInvalidInput = errors.New("invalid favicon input")
