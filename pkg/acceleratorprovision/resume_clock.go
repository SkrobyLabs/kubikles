package acceleratorprovision

import (
	"context"
	"time"
)

type resumeClock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type processResumeClock struct{}

func (processResumeClock) Now() time.Time { return time.Now() }

func (processResumeClock) Sleep(ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		return context.Canceled
	}
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
