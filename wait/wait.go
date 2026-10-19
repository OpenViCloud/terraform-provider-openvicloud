// Package wait polls until something the API reports is done, with a clock the caller passes in so tests never
// wait for real.
package wait

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout is returned when the deadline passes before check reports done.
var ErrTimeout = errors.New("timed out")

// Clock is the time source of a wait: the real one in main, a fake one in tests.
type Clock struct {
	Now   func() time.Time
	Sleep func(time.Duration)
}

// Until calls check, then sleeps interval and calls it again, until check reports done, check fails, ctx is
// cancelled, or timeout has passed since the first call.
func Until(ctx context.Context, clock Clock, timeout time.Duration, interval time.Duration, check func(ctx context.Context) (bool, error)) error {
	deadline := clock.Now().Add(timeout)
	for {
		done, err := check(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !clock.Now().Before(deadline) {
			return ErrTimeout
		}
		clock.Sleep(interval)
	}
}
