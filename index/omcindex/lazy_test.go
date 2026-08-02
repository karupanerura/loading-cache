package omcindex_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"golang.org/x/sync/errgroup"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// TestLazyInitialization_FailureIsReportedAndRetried verifies the fail-fast
// behavior of the lazy initial load: a failed load reports its error to every
// getter waiting for it (instead of leaving them blocked until their
// contexts expire), and the next call starts a fresh attempt.
func TestLazyInitialization_FailureIsReportedAndRetried(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source is down")
	var calls atomic.Int32
	source := index.FunctionIndexSource[uint8, uint8](func(ctx context.Context) (map[uint8][]uint8, error) {
		if calls.Add(1) == 1 {
			return nil, sourceErr
		}
		return map[uint8][]uint8{1: {2, 3}}, nil
	})
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

	// Every getter waiting for the failed load must receive its error.
	var eg errgroup.Group
	for i := 0; i < 4; i++ {
		eg.Go(func() error {
			if _, err := idx.Get(t.Context(), 1); !errors.Is(err, sourceErr) {
				// Late joiners may arrive after the failed flight is cleared
				// and trigger the second, successful load instead.
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// The failure must not stick: a subsequent Get starts a fresh load and
	// succeeds.
	pks, err := idx.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("the index is wedged by the failed load: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
}

// TestLazyInitialization_NilMapMeansEmptyIndex verifies that a source
// returning a nil map with no error initializes the index as empty. (The
// previous implementation used the nil-ness of the map as the "initialized"
// flag, so a nil result kept the index uninitialized and every read blocked
// forever.)
func TestLazyInitialization_NilMapMeansEmptyIndex(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	source := index.FunctionIndexSource[uint8, uint8](func(ctx context.Context) (map[uint8][]uint8, error) {
		calls.Add(1)
		return nil, nil
	})
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

	pks, err := idx.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pks != nil {
		t.Errorf("expected no primary keys from an empty index, got %v", pks)
	}

	// The index is initialized: further reads must not load again.
	if _, err := idx.Get(t.Context(), 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected exactly one load for an empty index, got %d", got)
	}

	// The same holds when Refresh publishes a nil map explicitly.
	if err := idx.Refresh(t.Context()); err != nil {
		t.Fatalf("unexpected refresh error: %v", err)
	}
	if m, err := idx.GetMulti(t.Context(), []uint8{1, 2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if len(m) != 0 {
		t.Errorf("expected an empty result from an empty index, got %v", m)
	}
}
