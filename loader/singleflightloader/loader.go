package singleflightloader

import (
	"context"
	"errors"
	"runtime"
	"sync"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/panicutil"
	"github.com/karupanerura/loading-cache/internal/sourceutil"
)

var errGoexit = errors.New("runtime.Goexit is called")

// SingleFlightLoader is a SourceLoader implementation that uses a single flight mechanism to load values.
// It uses a source to load the values, a storage to cache the values, and a cloner to clone the values.
type SingleFlightLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	storage loadingcache.CacheStorage[K, V]
	source  loadingcache.LoadingSource[K, V]
	cloner  loadingcache.ValueCloner[V]
	context func() context.Context

	mu        sync.Mutex
	waitlists map[K][]waiter[K, V]
}

var _ loadingcache.SourceLoader[uint8, struct{}] = (*SingleFlightLoader[uint8, struct{}])(nil)

// NewSingleFlightLoader creates a new SingleFlightLoader instance.
func NewSingleFlightLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](storage loadingcache.CacheStorage[K, V], source loadingcache.LoadingSource[K, V], opts ...Option[K, V]) *SingleFlightLoader[K, V] {
	loader := &SingleFlightLoader[K, V]{
		storage:   storage,
		source:    source,
		cloner:    nil,
		context:   context.Background,
		waitlists: map[K][]waiter[K, V]{},
	}
	for _, o := range opts {
		o.apply(loader)
	}
	if loader.cloner == nil {
		loader.cloner = loadingcache.DefaultValueCloner[V]()
	}
	return loader
}

// result is the outcome of a load for one input position of a call.
type result[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	pos   int
	err   error
	entry *loadingcache.Entry[K, V]
}

// waiter is an input position of a call waiting for a load of its key.
type waiter[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	results chan<- result[K, V]
	pos     int
}

// call is a registered LoadAndStore or LoadAndStoreMulti call.
// Its channel buffers one result per input position, and each position
// receives exactly one result, so loads never block on sending even after
// the caller stopped waiting, and the channel never needs to be closed.
type call[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	results chan result[K, V]
	size    int
}

func newCall[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](size int) *call[K, V] {
	return &call[K, V]{results: make(chan result[K, V], size), size: size}
}

// LoadAndStore retrieves a value associated with the given key from the source,
// stores it in the storage with an expiration time, and returns the value.
// The value is copied with the cloner unless this caller is the last waiter
// of a single-key load, which receives the value returned by the source.
// If an error occurs during retrieval or storage, it returns nil and the error.
// An already-canceled context returns its error without registering a load.
// After registration, canceling a caller's wait does not cancel the shared load.
// If a source, storage, cloner, or context provider callback calls runtime.Goexit during the shared load,
// a waiting caller that receives that notification also calls runtime.Goexit.
func (l *SingleFlightLoader[K, V]) LoadAndStore(ctx context.Context, key K) (*loadingcache.Entry[K, V], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := l.await(ctx, l.registerKey(key))
	if err != nil {
		return nil, err
	}
	return entries[0], nil
}

// registerKey registers a key and returns the call to receive the result.
func (l *SingleFlightLoader[K, V]) registerKey(key K) *call[K, V] {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := newCall[K, V](1)
	l.waitlists[key] = append(l.waitlists[key], waiter[K, V]{results: c.results, pos: 0})
	if len(l.waitlists[key]) == 1 {
		go l.loadKeyAndStore(key)
	}
	return c
}

// loadKeyAndStore loads a value from the source and stores it in the storage.
func (l *SingleFlightLoader[K, V]) loadKeyAndStore(key K) {
	l.load([]K{key}, func(ctx context.Context) ([]*loadingcache.CacheEntry[K, V], error) {
		entry, err := l.source.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if err := sourceutil.ValidateEntry(key, entry); err != nil {
			return nil, err
		}
		entries := []*loadingcache.CacheEntry[K, V]{entry}
		if entry == nil {
			return entries, nil
		}
		return entries, l.storage.Set(ctx, entry)
	})
}

// entriesForWaiters finishes cloning before any result is sent.
// If a cloner panics or calls Goexit, all waiters for this key receive the
// failure instead, and no waiter receives both a value and the failure.
func (l *SingleFlightLoader[K, V]) entriesForWaiters(cacheEntry *loadingcache.CacheEntry[K, V], count int, transferOriginal bool) []*loadingcache.Entry[K, V] {
	entries := make([]*loadingcache.Entry[K, V], count)
	if cacheEntry == nil || cacheEntry.NegativeCache {
		return entries
	}
	for i := range entries {
		entry := cacheEntry.Entry
		if !transferOriginal || i != count-1 {
			entry.Value = l.cloner.CloneValue(entry.Value)
		}
		entries[i] = &entry
	}
	return entries
}

// LoadAndStoreMulti loads multiple entries from the source using the provided keys,
// stores them in the cache, and returns the loaded entries. If an error occurs during
// the loading or storing process, it returns the error.
// An already-canceled context returns its error without registering any loads.
// After registration, canceling a caller's wait does not cancel the shared loads.
//
// The keys may be served by several shared loads, including loads started by
// other callers. The call returns the entries after results for all input
// positions arrive, or returns the first error it receives without waiting for
// the remaining loads, which continue in the background. Which error is returned
// when several loads fail depends on the order in which the call receives them,
// and when an error and the context's completion are both available, either may be returned.
// If a source, storage, cloner, or context provider callback calls runtime.Goexit during a shared load,
// a waiting caller that receives that notification also calls runtime.Goexit.
func (l *SingleFlightLoader[K, V]) LoadAndStoreMulti(ctx context.Context, keys []K) ([]*loadingcache.Entry[K, V], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.await(ctx, l.registerKeys(keys))
}

// await receives the results for all input positions of the call and returns
// the entries in input order. It returns on the first error it receives.
// Results sent after it returns stay in the call's buffer, so it needs no drainer.
func (l *SingleFlightLoader[K, V]) await(ctx context.Context, c *call[K, V]) ([]*loadingcache.Entry[K, V], error) {
	entries := make([]*loadingcache.Entry[K, V], c.size)
	for range c.size {
		select {
		case r := <-c.results:
			if r.err != nil {
				if r.err == errGoexit {
					runtime.Goexit()
				}
				return nil, r.err
			}
			entries[r.pos] = r.entry
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return entries, nil
}

// registerKeys registers keys and returns the call to receive the results.
func (l *SingleFlightLoader[K, V]) registerKeys(keys []K) *call[K, V] {
	l.mu.Lock()
	defer l.mu.Unlock()

	targetKeys := make([]K, 0, len(keys))
	c := newCall[K, V](len(keys))
	for i, key := range keys {
		l.waitlists[key] = append(l.waitlists[key], waiter[K, V]{results: c.results, pos: i})
		if len(l.waitlists[key]) == 1 {
			targetKeys = append(targetKeys, key)
		}
	}
	if len(targetKeys) != 0 {
		go l.loadKeysAndStore(targetKeys)
	}
	return c
}

// loadKeysAndStore loads values from the source and stores them in the storage.
func (l *SingleFlightLoader[K, V]) loadKeysAndStore(keys []K) {
	l.load(keys, func(ctx context.Context) ([]*loadingcache.CacheEntry[K, V], error) {
		entries, err := l.source.GetMulti(ctx, keys)
		if err != nil {
			return nil, err
		}
		if err := sourceutil.ValidateEntries(keys, entries); err != nil {
			return nil, err
		}
		return entries, l.storage.SetMulti(ctx, entries)
	})
}

// load protects all user callbacks, from the context provider through cloning.
// Each waiter receives exactly one result, including on panic or Goexit.
func (l *SingleFlightLoader[K, V]) load(keys []K, fetchAndStore func(context.Context) ([]*loadingcache.CacheEntry[K, V], error)) {
	var waitlists [][]waiter[K, V]
	var entries [][]*loadingcache.Entry[K, V]
	complete := func(err error) {
		if waitlists == nil {
			waitlists = l.detachWaitlists(keys)
		}
		for i, waitlist := range waitlists {
			for j, w := range waitlist {
				r := result[K, V]{pos: w.pos, err: err}
				if err == nil {
					r.entry = entries[i][j]
				}
				// The call's buffer has room for one result per position.
				w.results <- r
			}
		}
	}
	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() { complete(errGoexit) },
	}
	err := dds.Invoke(func() error {
		cacheEntries, err := fetchAndStore(l.context())
		if err != nil {
			return err
		}
		waitlists = l.detachWaitlists(keys)
		// New loads may now register the same keys. Completion must use
		// these captured waitlists, including if cloning panics or Goexits.
		entries = make([][]*loadingcache.Entry[K, V], len(keys))
		for i, waitlist := range waitlists {
			// Values may share mutable fields across keys, so transfer an
			// original only when the source was asked for a single key.
			entries[i] = l.entriesForWaiters(cacheEntries[i], len(waitlist), len(keys) == 1)
		}
		return nil
	})
	// If a user callback calls Goexit, Invoke never returns; OnGoexit completes the load instead.
	complete(err)
}

// detachWaitlists transfers ownership of the waitlists to the loading goroutine.
func (l *SingleFlightLoader[K, V]) detachWaitlists(keys []K) [][]waiter[K, V] {
	l.mu.Lock()
	defer l.mu.Unlock()
	waitlists := make([][]waiter[K, V], len(keys))
	for i, k := range keys {
		waitlists[i] = l.waitlists[k]
		delete(l.waitlists, k)
	}
	return waitlists
}
