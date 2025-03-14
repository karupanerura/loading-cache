package ctxsync_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/karupanerura/loading-cache/internal/ctxsync"
)

func TestCtxSyncCond(t *testing.T) {
	t.Parallel()

	t.Run("WaitCtx", func(t *testing.T) {
		t.Parallel()

		// Create a mutex and condition
		mu := &sync.Mutex{}
		cond := sync.NewCond(mu)
		ctxCond := &ctxsync.CtxSyncCond{Cond: cond}

		// Test with canceled context
		t.Run("CanceledContext", func(t *testing.T) {
			mu.Lock()

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel the context immediately

			// NOTE: mu.Unlock() will be called in the WaitCtx function asynchronously
			if err := ctxCond.WaitCtx(ctx); !errors.Is(err, context.Canceled) {
				t.Errorf("WaitCtx did not return expected error for canceled context: got %v, want %v", err, context.Canceled)
			}

			// Ensure the mutex is unlocked
			mu.Lock()
			cond.Signal()
			mu.Unlock()
		})

		// Test with valid context that completes successfully
		t.Run("SuccessfulWait", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			done := make(chan struct{})

			go func() {
				// Give time for the WaitCtx call to be set up
				time.Sleep(100 * time.Millisecond)

				// Signal the condition
				mu.Lock()
				cond.Signal()
				mu.Unlock()
			}()

			go func() {
				mu.Lock()
				if err := ctxCond.WaitCtx(ctx); err != nil {
					t.Errorf("WaitCtx failed with valid context: %v", err)
				}
				close(done)
			}()

			// Wait for WaitCtx to complete or timeout
			select {
			case <-done:
				// Success, WaitCtx completed
			case <-time.After(2 * time.Second):
				t.Error("WaitCtx did not complete within expected time")
			}

			// Ensure the mutex is unlocked
			mu.Unlock()
		})

		// Test context cancellation while waiting
		t.Run("CancellationDuringWait", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})

			go func() {
				mu.Lock()
				err := ctxCond.WaitCtx(ctx)
				if !errors.Is(err, context.Canceled) {
					t.Errorf("WaitCtx did not return expected error: got %v, want %v", err, context.Canceled)
				}
				close(done)
			}()

			// Wait a bit before canceling
			time.Sleep(100 * time.Millisecond)
			cancel()

			// Wait for WaitCtx to complete or timeout
			select {
			case <-done:
				// Success, WaitCtx completed with cancellation
			case <-time.After(2 * time.Second):
				t.Error("WaitCtx did not respond to cancellation within expected time")
			}

			// Ensure the mutex is unlocked
			mu.Lock()
			cond.Signal()
			mu.Unlock()
		})
	})
}
