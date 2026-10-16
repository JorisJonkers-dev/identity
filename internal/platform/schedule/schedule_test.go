package schedule_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/schedule"
)

func TestAJobRunsAtOnceAndThenEveryInterval(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		var runs atomic.Int32
		go schedule.Every(ctx, time.Hour, func(context.Context) error { runs.Add(1); return nil }, func(error) {})
		synctest.Wait()
		first := runs.Load()
		time.Sleep(2*time.Hour + time.Minute)
		synctest.Wait()
		cancel()
		if first != 1 || runs.Load() != 3 {
			t.Fatalf("ran %d at once and %d after two hours", first, runs.Load())
		}
	})
}

func TestAFailedRunIsReportedAndTheNextGoesAhead(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		boom := errors.New("boom")
		var reported, runs atomic.Int32
		go schedule.Every(ctx, time.Hour, func(context.Context) error { runs.Add(1); return boom }, func(err error) {
			if errors.Is(err, boom) {
				reported.Add(1)
			}
		})
		time.Sleep(time.Hour + time.Minute)
		synctest.Wait()
		cancel()
		if runs.Load() != 2 || reported.Load() != 2 {
			t.Fatalf("ran %d, reported %d", runs.Load(), reported.Load())
		}
	})
}

func TestARunCutShortByShutdownIsNotReported(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		var reported atomic.Int32
		done := make(chan struct{})
		go func() {
			schedule.Every(ctx, time.Hour, func(ctx context.Context) error { cancel(); return ctx.Err() }, func(error) { reported.Add(1) })
			close(done)
		}()
		<-done
		if reported.Load() != 0 {
			t.Fatal("a shutdown was reported as a failure")
		}
	})
}
