package utils

import (
	"context"
	"time"
)

var sleep = time.Sleep

func WaitFor(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sleep(d)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
