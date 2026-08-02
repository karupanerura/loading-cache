package singleflightloader

import (
	"context"
	"errors"
	"runtime"
	"sync"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/panicutil"
)

var errGoexit = errors.New("runtime.Goexit is called")

// SingleFlightLoader is a SourceLoader implementation that uses a single flight mechanism to load values.
// It uses a source to load the values, a storage to cache the values, and a cloner to clone the values.
type SingleFlightLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	storage loadingcache.CacheStorage[K, V]
	source  loadingcache.LoadingSource[K, V]
	cloner  loadingcache.ValueCloner[V]
	context func() context.Context

	mu        sync.RWMutex
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
// stores it in the storage with an expiration time, and returns the cloned value.
// If an error occurs during retrieval or storage, it returns the zero value of V and the error.
func (l *SingleFlightLoader[K, V]) LoadAndStore(ctx context.Context, key K) (*loadingcache.Entry[K, V], error) {
	ch := l.registerKey(ctx, key)
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
		go func() {
			<-ch
		}()
		return nil, ctx.Err()
	}
}

// registerKey registers a key and returns a channel to receive the result.
func (l *SingleFlightLoader[K, V]) registerKey(ctx context.Context, key K) chan either[error, *loadingcache.Entry[K, V]] {
	l.mu.Lock()
	defer l.mu.Unlock()

	ch := make(chan either[error, *loadingcache.Entry[K, V]], 1)
	l.waitlists[key] = append(l.waitlists[key], ch)
	if len(l.waitlists[key]) == 1 {
		// l.context() is a user-provided function: call it in the loading
		// goroutine, outside the mutex.
		go func() { l.loadKeyAndStore(l.context(), key) }()
	}
	return ch
}

// loadKeyAndStore loads a value from the source and stores it in the storage.
func (l *SingleFlightLoader[K, V]) loadKeyAndStore(ctx context.Context, key K) {
	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() {
			l.throwError(key, errGoexit)
		},
	}

	// storage.Set must also run inside the double defer sandwich: if it calls
	// runtime.Goexit (e.g. t.Fatal in a test storage), the waiters must be
	// notified. Otherwise the waitlist entry would survive forever and every
	// future call for the same key would join it without starting a new load.
	var cacheEntry *loadingcache.CacheEntry[K, V]
	if err := dds.Invoke(func() (err error) {
		cacheEntry, err = l.source.Get(ctx, key)
		if err != nil || cacheEntry == nil {
			return
		}
		err = l.storage.Set(ctx, cacheEntry)
		return
	}); err != nil {
		l.throwError(key, err)
		return
	}
	l.sendEntry(key, cacheEntry)
}

// sendEntry sends the entry to the waiting channels.
func (l *SingleFlightLoader[K, V]) sendEntry(key K, cacheEntry *loadingcache.CacheEntry[K, V]) {
	l.mu.Lock()
	waitlist := l.waitlists[key]
	// delete (not truncate) so that the map does not grow with every key ever loaded
	delete(l.waitlists, key)
	l.mu.Unlock()

	// The waitlist is detached from the map above, so no other goroutine can
	// reach it anymore: cloning and distribution run outside the mutex. This
	// keeps a slow user-provided cloner from serializing loads of unrelated
	// keys, and keeps a cloner that reaches back into this loader from
	// deadlocking on the mutex.
	for i, wl := range waitlist {
		if cacheEntry == nil || cacheEntry.NegativeCache {
			wl <- either[error, *loadingcache.Entry[K, V]]{R: nil}
		} else {
			entry := cacheEntry.Entry
			if i != len(waitlist)-1 {
				// note: we clone the value for each receiver except the last one
				// to avoid unnecessary cloning when there are multiple receivers.
				// Only the last receiver may take the original value: an earlier
				// receiver starts using (and possibly mutating) its entry as soon
				// as it is sent, while the original value is still being read here
				// to produce the clones for the remaining receivers.
				entry.Value = l.cloner.CloneValue(entry.Value)
			}
			wl <- either[error, *loadingcache.Entry[K, V]]{R: &entry}
		}
		close(wl)
	}
}

// throwError sends an error to the waiting channels.
func (l *SingleFlightLoader[K, V]) throwError(k K, err error) {
	l.mu.Lock()
	waitlist := l.waitlists[k]
	delete(l.waitlists, k)
	l.mu.Unlock()

	for _, wl := range waitlist {
		wl <- either[error, *loadingcache.Entry[K, V]]{L: err}
		close(wl)
	}
}

// LoadAndStoreMulti loads multiple entries from the source using the provided keys,
// stores them in the cache, and returns the loaded entries. If an error occurs during
// the loading or storing process, it returns the error.
func (l *SingleFlightLoader[K, V]) LoadAndStoreMulti(ctx context.Context, keys []K) ([]*loadingcache.Entry[K, V], error) {
	channels := l.registerKeys(ctx, keys)
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
			offset := i
			go func() {
				for _, ch := range channels[offset:] {
					<-ch
				}
			}()
			return nil, ctx.Err()
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return entries, nil
}

// registerKeys registers keys and returns channels to receive the results.
func (l *SingleFlightLoader[K, V]) registerKeys(ctx context.Context, keys []K) []chan either[error, *loadingcache.Entry[K, V]] {
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
		// l.context() is a user-provided function: call it in the loading
		// goroutine, outside the mutex.
		go func() { l.loadKeysAndStore(l.context(), targetKeys) }()
	}
	return channels
}

// loadKeysAndStore loads values from the source and stores them in the storage.
func (l *SingleFlightLoader[K, V]) loadKeysAndStore(ctx context.Context, keys []K) {
	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() {
			l.throwErrors(keys, errGoexit)
		},
	}

	// storage.SetMulti must also run inside the double defer sandwich: if it
	// calls runtime.Goexit (e.g. t.Fatal in a test storage), the waiters must
	// be notified. Otherwise the waitlist entries would survive forever and
	// every future call for the same keys would join them without starting a
	// new load.
	var entries []*loadingcache.CacheEntry[K, V]
	if err := dds.Invoke(func() (err error) {
		entries, err = l.source.GetMulti(ctx, keys)
		if err != nil {
			return
		}
		err = l.storage.SetMulti(ctx, entries)
		return
	}); err != nil {
		l.throwErrors(keys, err)
		return
	}

	l.sendEntries(keys, entries)
}

// sendEntries sends the entries to the waiting channels.
func (l *SingleFlightLoader[K, V]) sendEntries(keys []K, cacheEntries []*loadingcache.CacheEntry[K, V]) {
	waitlists := l.detachWaitlists(keys)

	// The waitlists are detached from the map above: cloning and distribution
	// run outside the mutex for the same reasons as in sendEntry.
	for i, waitlist := range waitlists {
		cacheEntry := cacheEntries[i]
		for j, wl := range waitlist {
			if cacheEntry == nil || cacheEntry.NegativeCache {
				wl <- either[error, *loadingcache.Entry[K, V]]{R: nil}
			} else {
				entry := cacheEntry.Entry
				if j != len(waitlist)-1 {
					// note: we clone the value for each receiver except the last one
					// to avoid unnecessary cloning when there are multiple receivers.
					// Only the last receiver may take the original value: an earlier
					// receiver starts using (and possibly mutating) its entry as soon
					// as it is sent, while the original value is still being read here
					// to produce the clones for the remaining receivers.
					entry.Value = l.cloner.CloneValue(entry.Value)
				}
				wl <- either[error, *loadingcache.Entry[K, V]]{R: &entry}
			}
			close(wl)
		}
	}
}

// throwErrors sends an error to the waiting channels.
func (l *SingleFlightLoader[K, V]) throwErrors(keys []K, err error) {
	waitlists := l.detachWaitlists(keys)

	for _, waitlist := range waitlists {
		for _, wl := range waitlist {
			wl <- either[error, *loadingcache.Entry[K, V]]{L: err}
			close(wl)
		}
	}
}

// detachWaitlists removes the waitlists for the given keys from the map and
// returns them in key order. Once detached, a waitlist is owned exclusively
// by the caller and can be worked on outside the mutex.
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
