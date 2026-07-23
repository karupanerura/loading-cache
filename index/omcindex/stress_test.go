package omcindex_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// TestStress_ConcurrentRefreshAndGet hammers OnMemoryIndex with concurrent
// Refresh, Get and GetMulti calls, mixing canceled contexts. Every getter
// asserts the result derived from the secondary key and then corrupts the
// returned slice in place: if the index handed out its internal slice instead
// of a copy, a subsequent getter (or the race detector) would catch it.
func TestStress_ConcurrentRefreshAndGet(t *testing.T) {
	t.Parallel()

	const (
		numSecondaryKeys = 8
		numGetters       = 6
		numIterations    = 300
		numRefreshes     = 30
	)

	buildIndex := func(context.Context) (map[uint8][]uint8, error) {
		time.Sleep(200 * time.Microsecond) // widen the refresh window
		m := make(map[uint8][]uint8, numSecondaryKeys)
		for sk := uint8(0); sk < numSecondaryKeys; sk++ {
			m[sk] = []uint8{sk * 2, sk*2 + 1}
		}
		return m, nil
	}
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](index.FunctionIndexSource[uint8, uint8](buildIndex))

	// Getters canceled before the first refresh must return the context error
	// instead of hanging, and must not leak the read lock.
	var eg errgroup.Group
	for i := 0; i < 4; i++ {
		eg.Go(func() error {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()
			if _, err := idx.Get(ctx, 1); err != context.DeadlineExceeded {
				return fmt.Errorf("expected context.DeadlineExceeded before the first refresh, got %v", err)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// A refresh after the canceled getters must not deadlock on a leaked read lock.
	refreshDone := make(chan error, 1)
	go func() {
		refreshDone <- idx.Refresh(t.Context())
	}()
	select {
	case err := <-refreshDone:
		if err != nil {
			t.Fatalf("unexpected refresh error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh deadlocked: a canceled getter leaked the read lock")
	}

	verify := func(sk uint8, pks []uint8) error {
		if sk < numSecondaryKeys {
			if len(pks) != 2 || pks[0] != sk*2 || pks[1] != sk*2+1 {
				return fmt.Errorf("unexpected primary keys for %d: %v (the internal slice leaked to another caller)", sk, pks)
			}
			// Corrupt the returned slice in place. If it is the index's
			// internal slice, a subsequent getter or the race detector will
			// catch it.
			pks[0], pks[1] = 255, 255
		} else if pks != nil {
			return fmt.Errorf("unexpected primary keys for unknown key %d: %v", sk, pks)
		}
		return nil
	}

	eg = errgroup.Group{}
	eg.Go(func() error {
		for i := 0; i < numRefreshes; i++ {
			if err := idx.Refresh(t.Context()); err != nil {
				return err
			}
			time.Sleep(100 * time.Microsecond)
		}
		return nil
	})
	for g := 0; g < numGetters; g++ {
		eg.Go(func() error {
			for i := 0; i < numIterations; i++ {
				// A secondary key beyond the source range verifies the not-found path.
				sk := uint8(rand.IntN(numSecondaryKeys + 1))
				if i%2 == 0 {
					pks, err := idx.Get(t.Context(), sk)
					if err != nil {
						return err
					}
					if err := verify(sk, pks); err != nil {
						return err
					}
				} else {
					sks := []uint8{sk, uint8(rand.IntN(numSecondaryKeys + 1)), sk} // duplicates are intentional
					m, err := idx.GetMulti(t.Context(), sks)
					if err != nil {
						return err
					}
					// verify each distinct key once: verify corrupts the slice in place
					seen := map[uint8]struct{}{}
					for _, sk := range sks {
						if _, ok := seen[sk]; ok {
							continue
						}
						seen[sk] = struct{}{}
						if err := verify(sk, m[sk]); err != nil {
							return err
						}
					}
				}

				// Exercise the canceled-waiter path from time to time.
				if i%32 == 31 {
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					if pks, err := idx.Get(ctx, sk); err == nil {
						// The lock may have been acquired before the cancellation was observed.
						if err := verify(sk, pks); err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// The index must still be consistent and refreshable after the churn.
	if err := idx.Refresh(t.Context()); err != nil {
		t.Fatalf("unexpected refresh error after the churn: %v", err)
	}
	pks, err := idx.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("unexpected error after the churn: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys after the churn: %v", pks)
	}
}
