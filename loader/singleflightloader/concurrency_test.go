package singleflightloader

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

type sharedValue struct {
	key int
	n   *int
}

func (v *sharedValue) Clone() *sharedValue {
	n := *v.n
	return &sharedValue{key: v.key, n: &n}
}

func TestLoadAndStoreMulti_SharedValuesAcrossKeys(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	src := source.GetMultiFunctionSource[int, *sharedValue](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, *sharedValue], error) {
		<-release
		// The entire batch belongs to the loader, but its values share a field.
		n := 42
		entries := make([]*loadingcache.CacheEntry[int, *sharedValue], len(keys))
		for i, key := range keys {
			entries[i] = &loadingcache.CacheEntry[int, *sharedValue]{
				Entry:     loadingcache.Entry[int, *sharedValue]{Key: key, Value: &sharedValue{key: key, n: &n}},
				ExpiresAt: time.Now().Add(time.Hour),
			}
		}
		return entries, nil
	})
	st := memstorage.NewInMemoryStorage[int, *sharedValue]()
	l := NewSingleFlightLoader(st, src)
	// Register overlapping single/multi requests before allowing the source
	// to return, including two receivers for the second key in one request.
	channels := l.registerKeys([]int{1, 2, 2})
	last := l.registerKey(1)
	close(release)
	entries, err := l.await(ctx, channels)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-last.results:
		if result.err != nil || result.entry == nil {
			t.Fatalf("single-key receiver: %+v", result)
		}
		*result.entry.Value.n = -1
	case <-ctx.Done():
		t.Fatal("single-key receiver did not finish")
	}
	for i, entry := range entries {
		if entry == nil || *entry.Value.n != 42 {
			t.Fatalf("result %d observed another receiver's mutation: %+v", i, entry)
		}
		*entry.Value.n = i
	}
	cached, err := st.GetMulti(ctx, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range cached {
		if entry == nil || *entry.Value.n != 42 {
			t.Errorf("cached result %d observed a receiver's mutation", i)
		}
	}
}

func TestLoadAndStoreMulti_SingleTargetClonesOnlyAdditionalWaiters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		waiters int
	}{{"OneWaiter", 1}, {"DuplicateKeys", 3}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			src := source.GetMultiFunctionSource[int, *sharedValue](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, *sharedValue], error) {
				n := 42
				return []*loadingcache.CacheEntry[int, *sharedValue]{{
					Entry:     loadingcache.Entry[int, *sharedValue]{Key: keys[0], Value: &sharedValue{key: keys[0], n: &n}},
					ExpiresAt: time.Now().Add(time.Hour),
				}}, nil
			})
			var clones atomic.Int32
			cloner := loadingcache.ValueClonerFunc[*sharedValue](func(v *sharedValue) *sharedValue {
				clones.Add(1)
				return v.Clone()
			})
			st := memstorage.NewInMemoryStorage[int, *sharedValue]()
			l := NewSingleFlightLoader(st, src, WithCloner[int](cloner))
			keys := make([]int, tc.waiters)
			entries, err := l.LoadAndStoreMulti(ctx, keys)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := clones.Load(), int32(tc.waiters-1); got != want {
				t.Errorf("loader clones = %d, want %d", got, want)
			}
			for i, entry := range entries {
				if entry == nil || *entry.Value.n != 42 {
					t.Fatalf("result %d observed another receiver's mutation: %+v", i, entry)
				}
				*entry.Value.n = -1
			}
			cached, err := st.Get(ctx, 0)
			if err != nil || cached == nil || *cached.Value.n != 42 {
				t.Fatalf("cached value changed: %+v, %v", cached, err)
			}
		})
	}
}

func TestClonerFailureNotifiesWaiters(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name     string
		multi    bool
		failAt   int32
		provider bool
	}{
		{name: "Single", failAt: 2},
		{name: "MultiFirstKey", multi: true, failAt: 2},
		{name: "MultiLaterKey", multi: true, failAt: 4},
		{name: "SingleContextProvider", provider: true},
		{name: "MultiContextProvider", multi: true, provider: true},
	} {
		for _, failure := range []string{"Goexit", "Panic"} {
			t.Run(scenario.name+"/"+failure, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				release := make(chan struct{})
				src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
					<-release
					entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
					for i, key := range keys {
						entries[i] = &loadingcache.CacheEntry[int, int]{
							Entry:     loadingcache.Entry[int, int]{Key: key, Value: 42},
							ExpiresAt: time.Now().Add(time.Hour),
						}
					}
					return entries, nil
				})
				var calls atomic.Int32
				failureErr := errors.New("cloning failed")
				fail := func() {
					if failure == "Goexit" {
						runtime.Goexit()
					}
					panic(failureErr)
				}
				cloner := loadingcache.ValueClonerFunc[int](func(v int) int {
					// Exercise failures after a successful clone and in a later key.
					if !scenario.provider && calls.Add(1) == scenario.failAt {
						fail()
					}
					return v
				})
				provider := func() context.Context {
					if scenario.provider && calls.Add(1) == 1 {
						<-release
						fail()
					}
					return context.Background()
				}
				l := NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src, WithCloner[int](cloner), WithBackgroundContextProvider[int, int](provider))
				var registered []*call[int, int]
				if scenario.multi {
					registered = append(registered, l.registerKeys([]int{1, 1, 1, 2, 2, 2}))
				} else {
					for range 3 {
						registered = append(registered, l.registerKey(1))
					}
				}
				close(release)
				for i, c := range registered {
					for range c.size {
						select {
						case result := <-c.results:
							want := failureErr
							if failure == "Goexit" {
								want = errGoexit
							}
							if !errors.Is(result.err, want) {
								t.Errorf("call %d position %d: got %v, want %v", i, result.pos, result.err, want)
							}
						case <-ctx.Done():
							t.Fatal("cloner failure left waiters blocked")
						}
					}
				}
				entries, err := l.LoadAndStoreMulti(ctx, []int{1, 2})
				if err != nil {
					t.Fatalf("retry after cloner failure: %v", err)
				}
				for i, entry := range entries {
					if entry == nil || entry.Value != 42 {
						t.Errorf("retry result %d: %+v", i, entry)
					}
				}
			})
		}
	}
}

func TestClonerFailureDoesNotAffectNewLoad(t *testing.T) {
	t.Parallel()
	for _, goexit := range []bool{false, true} {
		name := "Panic"
		if goexit {
			name = "Goexit"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cloneStarted := make(chan struct{})
			failRelease := make(chan struct{})
			failNow := sync.OnceFunc(func() { close(failRelease) })
			defer failNow()
			newStarted := make(chan struct{})
			newRelease := make(chan struct{})
			finishNew := sync.OnceFunc(func() { close(newRelease) })
			defer finishNew()
			var sourceCalls, cloneCalls atomic.Int32
			src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
				if sourceCalls.Add(1) == 2 {
					close(newStarted)
					<-newRelease
				}
				return []*loadingcache.CacheEntry[int, int]{{Entry: loadingcache.Entry[int, int]{Key: keys[0], Value: 42}, ExpiresAt: time.Now().Add(time.Hour)}}, nil
			})
			failureErr := errors.New("old load's clone failed")
			cloner := loadingcache.ValueClonerFunc[int](func(v int) int {
				if cloneCalls.Add(1) == 1 {
					close(cloneStarted)
					<-failRelease
					if goexit {
						runtime.Goexit()
					}
					panic(failureErr)
				}
				return v
			})
			l := NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src, WithCloner[int](cloner))
			oldChannels := l.registerKeys([]int{1, 1})
			select {
			case <-cloneStarted:
			case <-ctx.Done():
				t.Fatal("old cloner did not start")
			}
			registered := make(chan *call[int, int], 1)
			go func() { registered <- l.registerKeys([]int{1}) }()
			var newChannels *call[int, int]
			select {
			case newChannels = <-registered:
			case <-ctx.Done():
				t.Fatal("new load blocked by old cloner")
			}
			select {
			case <-newStarted:
			case <-ctx.Done():
				t.Fatal("new load joined the old waitlist")
			}
			failNow()
			for range oldChannels.size {
				select {
				case result := <-oldChannels.results:
					want := failureErr
					if goexit {
						want = errGoexit
					}
					if !errors.Is(result.err, want) {
						t.Errorf("old waiter: got %v, want %v", result.err, want)
					}
				case <-ctx.Done():
					t.Fatal("old waiter lost its failure notification")
				}
			}
			finishNew()
			entries, err := l.await(ctx, newChannels)
			if err != nil || len(entries) != 1 || entries[0] == nil || entries[0].Value != 42 {
				t.Fatalf("new load received the old load's failure: %v, %v", entries, err)
			}
		})
	}
}
