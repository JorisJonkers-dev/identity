package app

import (
	"sync"
	"time"
)

const (
	// sweepFloor is the fewest keys a limiter holds before it first forgets expired ones.
	sweepFloor = 1024
	// maxKeys bounds the keys a limiter holds. A full one refuses a new key rather than forget a
	// live one, which would let a flood of addresses reset another key's count.
	maxKeys = 50_000
)

// limiter counts recent hits per key over window, in memory: each replica counts its own. A key
// whose hits all left the window is forgotten once the limiter has grown enough to sweep.
type limiter struct {
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]time.Time
	// sweepAt is how many keys the limiter holds before a new key sweeps it.
	sweepAt int
	// expiring is the earliest a held key's last hit leaves its window, as of the last sweep.
	expiring time.Time
}

// allow records a hit for key and reports whether it is within most over the window.
func (l *limiter) allow(key string, most int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits, l.sweepAt = map[string][]time.Time{}, sweepFloor
	}
	held, known := l.hits[key]
	if !known && !l.room(now) {
		return false
	}
	recent := held[:0]
	for _, at := range held {
		if now.Sub(at) < l.window {
			recent = append(recent, at)
		}
	}
	recent = append(recent, now)
	l.hits[key] = recent
	return len(recent) <= most
}

// room reports whether the limiter can take one more key, first forgetting the keys whose hits all
// left the window once it holds sweepAt of them and one of them may have expired.
func (l *limiter) room(now time.Time) bool {
	if len(l.hits) >= l.sweepAt && !now.Before(l.expiring) {
		l.forget(now)
		l.sweepAt = min(max(2*len(l.hits), sweepFloor), maxKeys)
	}
	return len(l.hits) < maxKeys
}

func (l *limiter) forget(now time.Time) {
	l.expiring = time.Time{}
	for key, hits := range l.hits {
		expires := hits[len(hits)-1].Add(l.window)
		if !now.Before(expires) {
			delete(l.hits, key)
			continue
		}
		if l.expiring.IsZero() || expires.Before(l.expiring) {
			l.expiring = expires
		}
	}
}
