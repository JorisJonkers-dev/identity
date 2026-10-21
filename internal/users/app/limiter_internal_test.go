package app

import (
	"strconv"
	"testing"
	"time"
)

var start = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

const window = 15 * time.Minute

// flooded is a limiter that has seen n distinct keys, each once, at start.
func flooded(n int) *limiter {
	l := &limiter{window: window}
	for i := range n {
		l.allow("key:"+strconv.Itoa(i), 3, start)
	}
	return l
}

func TestALimiterForgetsKeysWhoseHitsAllLeftTheWindow(t *testing.T) {
	l := flooded(sweepFloor)

	l.allow("late", 3, start.Add(window))

	if len(l.hits) != 1 || l.hits["late"] == nil {
		t.Fatalf("the limiter holds %d keys", len(l.hits))
	}
}

func TestALimiterKeepsKeysWithAHitStillInsideTheWindow(t *testing.T) {
	l := flooded(sweepFloor)

	l.allow("late", 3, start.Add(window-time.Second))

	if len(l.hits) != sweepFloor+1 {
		t.Fatalf("the limiter holds %d keys", len(l.hits))
	}
}

func TestAFloodOfDistinctKeysNeverGrowsALimiterPastItsBound(t *testing.T) {
	l := flooded(maxKeys + 1_000)

	if len(l.hits) != maxKeys {
		t.Fatalf("the limiter holds %d keys", len(l.hits))
	}
}

func TestAFullLimiterRefusesANewKey(t *testing.T) {
	l := flooded(maxKeys)

	if l.allow("new", 3, start.Add(time.Minute)) {
		t.Fatal("a new key was let through a full limiter")
	}
}

func TestAFullLimiterStillCountsAKeyItHolds(t *testing.T) {
	l := flooded(maxKeys)

	if !l.allow("key:7", 3, start.Add(time.Minute)) || l.allow("key:7", 2, start.Add(time.Minute)) {
		t.Fatal("a held key was not counted as before")
	}
}

func TestAFullLimiterTakesNewKeysOnceItsOldOnesExpire(t *testing.T) {
	l := flooded(maxKeys)
	l.allow("new", 3, start.Add(time.Minute))

	if !l.allow("new", 3, start.Add(window)) || len(l.hits) != 1 {
		t.Fatalf("the limiter holds %d keys", len(l.hits))
	}
}

func TestAFullLimiterTakesANewKeyAsSoonAsItsEarliestKeyExpires(t *testing.T) {
	l := flooded(maxKeys - 2)
	l.allow("earlier", 3, start.Add(5*time.Minute))
	l.allow("later", 3, start.Add(10*time.Minute))
	for i := range maxKeys - 2 {
		l.allow("again:"+strconv.Itoa(i), 3, start.Add(window))
	}

	if !l.allow("new", 3, start.Add(5*time.Minute+window)) {
		t.Fatal("a new key was refused after the earliest key expired")
	}
}
