package sampler

import (
	"context"
	"time"
)

// retryAttempts and retryBaseDelay implement the sampler's retry policy: 3
// attempts total, exponential backoff starting at 200ms. Used for every
// cluster call so a single transient blip doesn't abort a whole sampling run.
const (
	retryAttempts  = 3
	retryBaseDelay = 200 * time.Millisecond
)

// withRetry runs fn up to retryAttempts times, backing off exponentially
// between attempts, and returns the last error if every attempt fails.
func withRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = fn(); err == nil {
			return nil
		}
	}
	return err
}
