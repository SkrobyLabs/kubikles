package store

import "time"

// Clock is the time source of the reflector and the manager. Tests inject a
// manual clock so backoff and linger run without sleeping.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	// AfterFunc runs f after d and returns a function that stops the timer and
	// reports whether it was stopped before firing.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (realClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}
