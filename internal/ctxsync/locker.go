package ctxsync

import (
	"context"
	"sync"
)

// CtxLocker is a wrapper of sync.Locker that can lock with context.
type CtxLocker struct {
	sync.Locker
}

// tryLocker is an interface for the TryLock method.
type tryLocker interface {
	TryLock() bool
}

// LockCtx try to lock with context.
// If the context is canceled before the lock is acquired, it returns the context error.
func (l *CtxLocker) LockCtx(ctx context.Context) error {
	if tl, ok := l.Locker.(tryLocker); ok && tl.TryLock() {
		return nil
	}

	lock := make(chan struct{})
	go func() {
		defer close(lock)
		l.Locker.Lock()
	}()

	select {
	case <-lock:
		return nil
	case <-ctx.Done():
		go func() {
			<-lock
			l.Unlock()
		}()
		return ctx.Err()
	}
}
