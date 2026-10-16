// Package schedule runs a job in the background on a fixed interval, for as long as a context lives.
package schedule

import (
	"context"
	"time"
)

// Every runs job at once and then every interval until ctx ends. A run that fails is reported to
// failed and the next run goes ahead; every replica runs it, so a job must be safe to repeat.
func Every(ctx context.Context, interval time.Duration, job func(context.Context) error, failed func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := job(ctx); err != nil && ctx.Err() == nil {
			failed(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
