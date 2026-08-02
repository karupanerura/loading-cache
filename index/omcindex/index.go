package omcindex

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/panicutil"
)

// OnMemoryIndex is an in-memory index that stores the mapping between secondary keys and primary keys.
//
// The index initializes itself lazily: the first Get/GetMulti triggers a
// single load from the source (concurrent callers share the same load) and
// waits for it. Refresh can still be called explicitly — typically via
// intervalupdater — to initialize the index eagerly and to update it
// afterwards; an eagerly initialized index never runs the lazy load.
type OnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	source  loadingcache.IndexSource[SecondaryKey, PrimaryKey]
	context func() context.Context

	// m is the published snapshot of the index. It stays nil until the first
	// successful load. A published map is never mutated afterwards: Refresh
	// replaces it wholesale, and the IndexSource.GetAll ownership contract
	// forbids the source from retaining and mutating it. Readers therefore
	// read the snapshot without holding any lock.
	m atomic.Pointer[map[SecondaryKey][]PrimaryKey]

	flightMu sync.Mutex
	flight   *initFlight // in-flight initial load; nil when none is running
}

var _ loadingcache.Index[uint8, uint8] = (*OnMemoryIndex[uint8, uint8])(nil)
var _ loadingcache.RefreshIndex = (*OnMemoryIndex[uint8, uint8])(nil)

// initFlight represents a single in-flight attempt to initialize the index.
type initFlight struct {
	done   chan struct{} // closed when the attempt finishes in any way
	err    error         // valid after done is closed
	goexit bool          // valid after done is closed
}

// NewOnMemoryIndex creates a new OnMemoryIndex instance.
func NewOnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint](source loadingcache.IndexSource[SecondaryKey, PrimaryKey], opts ...Option[SecondaryKey, PrimaryKey]) *OnMemoryIndex[SecondaryKey, PrimaryKey] {
	index := &OnMemoryIndex[SecondaryKey, PrimaryKey]{
		source:  source,
		context: context.Background,
	}
	for _, o := range opts {
		o.apply(index)
	}
	return index
}

// Refresh refreshes the index entries.
// It retrieves all the entries from the source and replaces the index
// snapshot atomically. Readers are never blocked by a refresh: they keep
// reading the previous snapshot until the new one is published.
// If an error occurs during retrieval, it returns the error and keeps the
// current snapshot.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Refresh(ctx context.Context) error {
	// The double defer sandwich turns a panic inside the source into an
	// error so that a background refresher (e.g. intervalupdater) does not
	// crash the process.
	var m map[SecondaryKey][]PrimaryKey
	if err := panicutil.DDS(func() (err error) {
		m, err = i.source.GetAll(ctx)
		return
	}); err != nil {
		return err
	}
	i.m.Store(&m)
	return nil
}

// Get retrieves primary keys by secondary key.
// If the index is not initialized yet, it triggers the initial load (shared
// with concurrent callers) and waits for it; the wait can be canceled through
// the context, and a failed load is reported as an error. Once the index is
// initialized, reads are lock-free and never block.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Get(ctx context.Context, sk SecondaryKey) ([]PrimaryKey, error) {
	for {
		if m := i.m.Load(); m != nil {
			pks := (*m)[sk]
			if pks == nil {
				return nil, nil
			}
			out := make([]PrimaryKey, len(pks))
			copy(out, pks)
			return out, nil
		}
		if err := i.awaitInitialization(ctx); err != nil {
			return nil, err
		}
	}
}

// GetMulti retrieves primary keys by multiple secondary keys.
// If the index is not initialized yet, it triggers the initial load (shared
// with concurrent callers) and waits for it; the wait can be canceled through
// the context, and a failed load is reported as an error. Once the index is
// initialized, reads are lock-free and never block.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, sks []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error) {
	for {
		if m := i.m.Load(); m != nil {
			result := make(map[SecondaryKey][]PrimaryKey, len(sks))
			for _, sk := range sks {
				if pks, ok := (*m)[sk]; ok {
					out := make([]PrimaryKey, len(pks))
					copy(out, pks)
					result[sk] = out
				}
			}
			return result, nil
		}
		if err := i.awaitInitialization(ctx); err != nil {
			return nil, err
		}
	}
}

// awaitInitialization joins (or starts) the in-flight initial load and waits
// for it to finish. It returns nil when the attempt succeeded (the caller
// re-reads the published snapshot), the attempt's error when it failed, or
// the context error when the caller gave up waiting. Giving up abandons no
// goroutine: the shared load keeps running for the remaining waiters and
// publishes its result when it finishes.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) awaitInitialization(ctx context.Context) error {
	i.flightMu.Lock()
	if m := i.m.Load(); m != nil {
		// initialized in the meantime (e.g. by a concurrent Refresh):
		// nothing to wait for
		i.flightMu.Unlock()
		return nil
	}
	f := i.flight
	if f == nil {
		f = &initFlight{done: make(chan struct{})}
		i.flight = f
		go i.runInitialLoad(f)
	}
	i.flightMu.Unlock()

	select {
	case <-f.done:
		if f.goexit {
			// Propagate runtime.Goexit to the waiters, following the same
			// symmetry rule as golang.org/x/sync/singleflight: any waiter
			// would have met the same fate had it been the executor.
			runtime.Goexit()
		}
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runInitialLoad performs one initialization attempt and publishes the result.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) runInitialLoad(f *initFlight) {
	// The deferred functions run in LIFO order: close(f.done) first, then the
	// flight slot is cleared. A Get arriving between the two joins the
	// just-finished flight and observes its result immediately instead of
	// starting a duplicate load; a Get arriving after the slot is cleared
	// either sees the published snapshot (on success) or starts a fresh
	// attempt (on failure), which makes failed initializations retryable.
	// Both deferred functions also run on runtime.Goexit, so a goexit-ed
	// attempt cannot wedge the index either.
	defer func() {
		i.flightMu.Lock()
		i.flight = nil
		i.flightMu.Unlock()
	}()
	defer close(f.done)

	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() { f.goexit = true },
	}
	var m map[SecondaryKey][]PrimaryKey
	if err := dds.Invoke(func() (err error) {
		m, err = i.source.GetAll(i.context())
		return
	}); err != nil {
		f.err = err
		return
	}
	i.m.Store(&m)
}
