package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type lifecycleFake struct {
	calls    []string
	clearErr error
}

func (f *lifecycleFake) ClearSessionAndWatcherState(context.Context) error {
	f.calls = append(f.calls, "clear")
	return f.clearErr
}
func (f *lifecycleFake) RequestProcessExit() { f.calls = append(f.calls, "exit") }

func TestAcceleratorIdleReconnectGrace(t *testing.T) {
	if AcceleratorIdleReconnectGrace != 2*time.Minute {
		t.Fatalf("grace = %v", AcceleratorIdleReconnectGrace)
	}
}
func TestExpireDisposableIdleOrdersCleanupBeforeExit(t *testing.T) {
	f := &lifecycleFake{}
	if err := ExpireDisposableIdle(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"clear", "exit"}) {
		t.Fatalf("calls = %#v", f.calls)
	}
}
func TestExpireDisposableIdleRequestsExitAfterCleanupError(t *testing.T) {
	sentinel := errors.New("cleanup")
	f := &lifecycleFake{clearErr: sentinel}
	if err := ExpireDisposableIdle(context.Background(), f); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if !reflect.DeepEqual(f.calls, []string{"clear", "exit"}) {
		t.Fatalf("calls = %#v", f.calls)
	}
}
