package omcindex

import (
	"context"
	"runtime"
	"sync"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/ctxsync"
	"github.com/karupanerura/loading-cache/internal/panicutil"
)

// OnMemoryIndex is an in-memory index that stores the mapping between secondary keys and primary keys.
type OnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	source loadingcache.IndexSource[SecondaryKey, PrimaryKey]

	mu     sync.RWMutex
	rl     ctxsync.CtxLocker
	sc     ctxsync.CtxSyncCond
	goexit bool
	m      map[SecondaryKey][]PrimaryKey
}

var _ loadingcache.Index[uint8, uint8] = (*OnMemoryIndex[uint8, uint8])(nil)
var _ loadingcache.RefreshIndex = (*OnMemoryIndex[uint8, uint8])(nil)

// NewOnMemoryIndex creates a new OnMemoryIndex instance.
func NewOnMemoryIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint](source loadingcache.IndexSource[SecondaryKey, PrimaryKey]) *OnMemoryIndex[SecondaryKey, PrimaryKey] {
	index := &OnMemoryIndex[SecondaryKey, PrimaryKey]{
		source: source,
	}
	index.rl = ctxsync.CtxLocker{Locker: index.mu.RLocker()}
	index.sc = ctxsync.CtxSyncCond{Cond: sync.NewCond(index.rl.Locker)}
	return index
}

// Refresh refreshes the index entries.
// It retrieves all the entries from the source and updates the index.
// If an error occurs during retrieval, it returns the error.
// This method is blocking any other calls until the index is refreshed.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Refresh(ctx context.Context) error {
	dds := panicutil.DoubleDeferSandwich{
		OnGoexit: func() {
			i.mu.Lock()
			defer i.mu.Unlock()

			i.goexit = true
			i.sc.Broadcast()
		},
	}

	var m map[SecondaryKey][]PrimaryKey
	if err := dds.Invoke(func() (err error) {
		m, err = i.source.GetAll(ctx)
		return
	}); err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	i.m = m
	i.sc.Broadcast()
	return nil
}

// Goexit calls the Goexit method from waiting for the refresh operation to complete.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Goexit() {
}

// Get retrieves primary keys by secondary key.
// This method is blocked until the index is initialized.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) Get(ctx context.Context, sk SecondaryKey) ([]PrimaryKey, error) {
	if err := i.rl.LockCtx(ctx); err != nil {
		return nil, err
	}
	for i.m == nil {
		if i.goexit {
			runtime.Goexit()
		}
		if err := i.sc.WaitCtx(ctx); err != nil {
			return nil, err
		}
	}
	defer i.rl.Unlock()

	if i.m[sk] == nil {
		return nil, nil
	}

	pks := make([]PrimaryKey, len(i.m[sk]))
	copy(pks, i.m[sk])
	return pks, nil
}

// GetMulti retrieves primary keys by multiple secondary keys.
// This method is blocked until the index is initialized.
func (i *OnMemoryIndex[SecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, sks []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error) {
	if err := i.rl.LockCtx(ctx); err != nil {
		return nil, err
	}
	for i.m == nil {
		if i.goexit {
			runtime.Goexit()
		}
		if err := i.sc.WaitCtx(ctx); err != nil {
			return nil, err
		}
	}
	defer i.rl.Unlock()

	m := make(map[SecondaryKey][]PrimaryKey, len(sks))
	for _, sk := range sks {
		if pks, ok := i.m[sk]; ok {
			m[sk] = make([]PrimaryKey, len(pks))
			copy(m[sk], pks)
		}
	}
	return m, nil
}
