package ctxsync

import (
	"context"
	"sync"
)

// CtxSyncCond is a wrapper of sync.Cond that can wait with context.
type CtxSyncCond struct {
	*sync.Cond
}

// WaitCtx waits to be notified or canceled.
// If the context is canceled before the condition is notified, it returns the context error.
func (c *CtxSyncCond) WaitCtx(ctx context.Context) error {
	lock := make(chan struct{})
	go func() {
		defer close(lock)
		c.Cond.Wait()
	}()

	select {
	case <-lock:
		return nil
	case <-ctx.Done():
		go func() {
			defer c.Cond.L.Unlock()
			<-lock
		}()
		return ctx.Err()
	}
}
