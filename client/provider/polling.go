package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// PollSettings supplies the shared timeout, cadence, and validation settings for polling.
type PollSettings struct {
	Interval, Timeout    time.Duration
	MaxTransientFailures int
}

// NewPollContext validates settings and applies the SDK polling defaults.
func NewPollContext(ctx context.Context, options PollSettings) (context.Context, context.CancelFunc, time.Duration, error) {
	if options.Interval < 0 || options.Timeout < 0 || options.MaxTransientFailures < 0 {
		return nil, nil, 0, errors.New("poll options must not be negative")
	}
	interval := options.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, interval, nil
}

// RetryDelay honors Retry-After before the configured backoff.
func RetryDelay(headers http.Header, backoff time.Duration, attempt int) time.Duration {
	if value := headers.Get(wire.HeaderRetryAfter); value != "" {
		var seconds int
		if _, err := fmt.Sscanf(value, "%d", &seconds); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			return max(time.Duration(0), time.Until(when))
		}
	}
	for i := 0; i < attempt && backoff < 30*time.Second; i++ {
		backoff *= 2
	}
	return min(backoff, 30*time.Second)
}

// WaitContext sleeps until the delay expires or the context ends.
func WaitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
