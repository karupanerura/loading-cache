package ctxsync_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/karupanerura/loading-cache/internal/ctxsync"
)

// withoutTryLocker implements a Locker without TryLock for testing
type withoutTryLocker struct {
	locker sync.Locker
}

func (m *withoutTryLocker) Lock()   { m.locker.Lock() }
func (m *withoutTryLocker) Unlock() { m.locker.Unlock() }

func TestCtxLocker(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		locker sync.Locker
	}{
		{"GenericCase", &withoutTryLocker{locker: &sync.Mutex{}}},
		{"TryLockOptimization", &sync.Mutex{}},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctxLocker := &ctxsync.CtxLocker{Locker: tt.locker}

			// Lock before call LockCtx
			tt.locker.Lock()

			// Lock should fail with a canceled context
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := ctxLocker.LockCtx(ctx); !errors.Is(err, context.Canceled) {
				t.Errorf("Lock did not return expected error for canceled context")
			}

			// Unlock for the next test
			tt.locker.Unlock()

			// Wait for the lock to be released before the next test
			time.Sleep(100 * time.Millisecond)
			tt.locker.Lock()
			tt.locker.Unlock()

			// Lock should succeed with a valid context when unlocked
			if err := ctxLocker.LockCtx(context.Background()); err != nil {
				t.Errorf("Lock failed: %v", err)
			}

			// Lock is waiting for the context to be canceled
			ctx, cancel = context.WithCancel(context.Background())
			ch := make(chan struct{})
			go func() {
				<-ch
				cancel()
			}()
			done := make(chan struct{})
			go func() {
				defer close(done)
				if err := ctxLocker.LockCtx(ctx); !errors.Is(err, context.Canceled) {
					t.Errorf("Lock did not return expected error for canceled context: got=%+v", err)
				}
			}()

			time.Sleep(100 * time.Millisecond)
			close(ch)
			<-done

			tt.locker.Unlock()
		})
	}
}
