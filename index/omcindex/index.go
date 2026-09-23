package omcindex

import (
	"context"
	"runtime"
	"sync/atomic"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/panicutil"
)

// OnMemoryIndex is an in-memory index that stores the mapping between secondary keys and primary keys.
// Create it with NewOnMemoryIndex; the zero value is not ready for use.
type OnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	source loadingcache.IndexSource[SecondaryKey, PrimaryKey]

	// refreshing holds a token while a Refresh retrieves and publishes data.
	// It serializes refreshes and lets waiting callers stop on cancellation.
	refreshing chan struct{}
	state      atomic.Pointer[snapshot[SecondaryKey, PrimaryKey]]
}

// A published snapshot is immutable. The source transfers ownership of its
// map and slices, and readers copy the slices before returning them.
type snapshot[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	m       map[SecondaryKey][]PrimaryKey
	goexit  bool
	changed chan struct{} // closed when this snapshot is replaced
}

var _ loadingcache.Index[uint8, uint8] = (*OnMemoryIndex[uint8, uint8])(nil)
var _ loadingcache.RefreshIndex = (*OnMemoryIndex[uint8, uint8])(nil)

// NewOnMemoryIndex creates a new OnMemoryIndex instance.
func NewOnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint](source loadingcache.IndexSource[SecondaryKey, PrimaryKey]) *OnMemoryIndex[SecondaryKey, PrimaryKey] {
	index := &OnMemoryIndex[SecondaryKey, PrimaryKey]{
		source:     source,
		refreshing: make(chan struct{}, 1),
	}
	index.state.Store(&snapshot[SecondaryKey, PrimaryKey]{changed: make(chan struct{})})
	return index
}

// Refresh refreshes the index entries.
// It retrieves all the entries from the source with ctx and publishes them as a new snapshot.
// Readers use the previous snapshot until the new one is published.
// A successful refresh initializes the index; a nil map represents an empty index.
// Retrieval errors and panics from the source, recovered as errors, are returned
// and leave the previous snapshot unchanged.
//
// Refresh calls run one at a time, and each call that is not canceled retrieves
// on its own, so N concurrent calls retrieve all the entries up to N times.
// The retrieval starts after the call is made. A call whose ctx is already done returns ctx's error
// without calling the source, and a call waiting for another refresh returns
// ctx's error when ctx is done. Once the retrieval starts, canceling ctx stops
// it only if the source honors ctx. The source must not call Refresh on the same
// index, because that call waits for the refresh that is calling the source.
//
// Retrievals and publications of different calls do not overlap, so an older
// retrieval never overwrites a newer snapshot. A successful call returns after
// its snapshot is published; a later Refresh may publish newer data before the
// caller reads the index.
//
// If the source calls runtime.Goexit, the calling goroutine exits. The Goexit
// does not propagate to other Refresh calls, which can run afterwards. While the
// index is uninitialized, readers propagate that Goexit until a later Refresh succeeds.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case i.refreshing <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	// Release on every exit, including a panic or Goexit in the source,
	// so that later calls can refresh.
	defer func() { <-i.refreshing }()
	// select may acquire the token even if ctx is done at the same time.
	if err := ctx.Err(); err != nil {
		return err
	}
	return i.refresh(ctx)
}

func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) refresh(ctx context.Context) error {
	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() {
			// A failed refresh must keep any existing data available, including
			// an empty index, so only an uninitialized index records the Goexit.
			if i.state.Load().m == nil {
				i.publish(&snapshot[SecondaryKey, PrimaryKey]{goexit: true, changed: make(chan struct{})})
			}
		},
	}

	var m map[SecondaryKey][]PrimaryKey
	if err := dds.Invoke(func() (err error) {
		m, err = i.source.GetAll(ctx)
		return
	}); err != nil {
		return err
	}

	if m == nil {
		m = make(map[SecondaryKey][]PrimaryKey)
	}
	i.publish(&snapshot[SecondaryKey, PrimaryKey]{m: m, changed: make(chan struct{})})
	return nil
}

// publish replaces the current snapshot with next and wakes the readers waiting for a change.
// It is called only while holding refreshing, which serializes the swap and the notification.
// The published map, its slices, and the snapshot must not be modified afterwards.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) publish(next *snapshot[SecondaryKey, PrimaryKey]) {
	previous := i.state.Swap(next)
	close(previous.changed)
}

func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) loadSnapshot(ctx context.Context) (map[SecondaryKey][]PrimaryKey, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := i.state.Load()
		if state.m != nil {
			return state.m, nil
		}
		if state.goexit {
			runtime.Goexit()
		}
		// Watching the channel from the same snapshot avoids a lost wakeup
		// if Refresh publishes between the state check and this select.
		select {
		case <-state.changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Get retrieves primary keys by secondary key.
// It waits for the first successful Refresh, unless canceled or a refresh calls Goexit.
// An already-canceled context takes precedence over an available snapshot or Goexit.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Get(ctx context.Context, sk SecondaryKey) ([]PrimaryKey, error) {
	m, err := i.loadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	pks := m[sk]
	if pks == nil {
		return nil, nil
	}

	result := make([]PrimaryKey, len(pks))
	copy(result, pks)
	return result, nil
}

// GetMulti retrieves primary keys by multiple secondary keys.
// It waits for the first successful Refresh, unless canceled or a refresh calls Goexit.
// An already-canceled context takes precedence over an available snapshot or Goexit.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, sks []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error) {
	m, err := i.loadSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[SecondaryKey][]PrimaryKey, len(sks))
	for _, sk := range sks {
		if pks, ok := m[sk]; ok {
			result[sk] = make([]PrimaryKey, len(pks))
			copy(result[sk], pks)
		}
	}
	return result, nil
}
