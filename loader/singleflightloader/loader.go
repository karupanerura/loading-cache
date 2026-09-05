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
	waitlists map[K][]chan either[error, *loadingcache.Entry[K, V]]
}

var _ loadingcache.SourceLoader[uint8, struct{}] = (*SingleFlightLoader[uint8, struct{}])(nil)

// NewSingleFlightLoader creates a new SingleFlightLoader instance.
func NewSingleFlightLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](storage loadingcache.CacheStorage[K, V], source loadingcache.LoadingSource[K, V], opts ...Option[K, V]) *SingleFlightLoader[K, V] {
	loader := &SingleFlightLoader[K, V]{
		storage:   storage,
		source:    source,
		cloner:    nil,
		context:   context.Background,
		waitlists: map[K][]chan either[error, *loadingcache.Entry[K, V]]{},
	}
	for _, o := range opts {
		o.apply(loader)
	}
	if loader.cloner == nil {
		loader.cloner = loadingcache.DefaultValueCloner[V]()
	}
	return loader
}

type either[L any, R any] struct {
	L L
	R R
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
	ch := l.registerKey(key)
	select {
	case e := <-ch:
		if e.L != nil {
			if e.L == errGoexit {
				runtime.Goexit()
			}
			return nil, e.L
		}
		return e.R, nil
	case <-ctx.Done():
		// Each channel buffers its sole result, so cancellation needs no drainer.
		return nil, ctx.Err()
	}
}

// registerKey registers a key and returns a channel to receive the result.
func (l *SingleFlightLoader[K, V]) registerKey(key K) chan either[error, *loadingcache.Entry[K, V]] {
	l.mu.Lock()
	defer l.mu.Unlock()

	ch := make(chan either[error, *loadingcache.Entry[K, V]], 1)
	l.waitlists[key] = append(l.waitlists[key], ch)
	if len(l.waitlists[key]) == 1 {
		go l.loadKeyAndStore(key)
	}
	return ch
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

// entriesForWaiters finishes cloning before any channel is sent or closed.
// If a cloner panics or calls Goexit, all waiters for this key can still
// receive the failure without sending to an already closed channel.
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
// If a source, storage, cloner, or context provider callback calls runtime.Goexit during a shared load,
// a waiting caller that receives that notification also calls runtime.Goexit.
func (l *SingleFlightLoader[K, V]) LoadAndStoreMulti(ctx context.Context, keys []K) ([]*loadingcache.Entry[K, V], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	channels := l.registerKeys(keys)
	return l.awaitChannels(ctx, channels)
}

// awaitChannels waits for the channels to receive the results and returns the entries.
func (l *SingleFlightLoader[K, V]) awaitChannels(ctx context.Context, channels []chan either[error, *loadingcache.Entry[K, V]]) ([]*loadingcache.Entry[K, V], error) {
	entries := make([]*loadingcache.Entry[K, V], len(channels))

	var lastErr error
	for i, ch := range channels {
		select {
		case e := <-ch:
			if e.L != nil {
				lastErr = e.L
				if e.L == errGoexit {
					runtime.Goexit()
				}
				continue
			}
			entries[i] = e.R
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return entries, nil
}

// registerKeys registers keys and returns channels to receive the results.
func (l *SingleFlightLoader[K, V]) registerKeys(keys []K) []chan either[error, *loadingcache.Entry[K, V]] {
	l.mu.Lock()
	defer l.mu.Unlock()

	targetKeys := make([]K, 0, len(keys))
	channels := make([]chan either[error, *loadingcache.Entry[K, V]], len(keys))
	for i, key := range keys {
		ch := make(chan either[error, *loadingcache.Entry[K, V]], 1)
		l.waitlists[key] = append(l.waitlists[key], ch)
		if len(l.waitlists[key]) == 1 {
			targetKeys = append(targetKeys, key)
		}
		channels[i] = ch
	}
	if len(targetKeys) != 0 {
		go l.loadKeysAndStore(targetKeys)
	}
	return channels
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
	var waitlists [][]chan either[error, *loadingcache.Entry[K, V]]
	var entries [][]*loadingcache.Entry[K, V]
	complete := func(err error) {
		if waitlists == nil {
			waitlists = l.detachWaitlists(keys)
		}
		for i, waitlist := range waitlists {
			for j, wl := range waitlist {
				result := either[error, *loadingcache.Entry[K, V]]{L: err}
				if err == nil {
					result.R = entries[i][j]
				}
				wl <- result
				close(wl)
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
func (l *SingleFlightLoader[K, V]) detachWaitlists(keys []K) [][]chan either[error, *loadingcache.Entry[K, V]] {
	l.mu.Lock()
	defer l.mu.Unlock()
	waitlists := make([][]chan either[error, *loadingcache.Entry[K, V]], len(keys))
	for i, k := range keys {
		waitlists[i] = l.waitlists[k]
		delete(l.waitlists, k)
	}
	return waitlists
}
