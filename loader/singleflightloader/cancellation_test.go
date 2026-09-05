package singleflightloader

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

func TestCanceledCallsDoNotRegister(t *testing.T) {
	t.Parallel()
	for _, multi := range []bool{false, true} {
		name := "Single"
		if multi {
			name = "Multi"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			release := make(chan struct{})
			defer close(release)
			src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
				<-release // Keep any incorrectly started load registered until the assertion.
				return make([]*loadingcache.CacheEntry[int, int], len(keys)), nil
			})
			l := NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src)
			var err error
			if multi {
				_, err = l.LoadAndStoreMulti(ctx, []int{1, 2})
			} else {
				_, err = l.LoadAndStore(ctx, 1)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want context.Canceled", err)
			}
			l.mu.Lock()
			defer l.mu.Unlock()
			if len(l.waitlists) != 0 {
				t.Fatal("an already-canceled call registered a load")
			}
		})
	}
}

func TestSlowClonerDoesNotBlockOtherKeys(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sourceRelease := make(chan struct{})
	cloneStarted := make(chan struct{})
	cloneRelease := make(chan struct{})
	finishClone := sync.OnceFunc(func() { close(cloneRelease) })
	defer finishClone()
	var once sync.Once
	src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
		<-sourceRelease
		entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
		for i, key := range keys {
			entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key}, ExpiresAt: time.Now().Add(time.Hour)}
		}
		return entries, nil
	})
	cloner := loadingcache.ValueClonerFunc[int](func(v int) int {
		if v == 1 {
			once.Do(func() { close(cloneStarted) })
			<-cloneRelease
		}
		return v
	})
	l := NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src, WithCloner[int](cloner))
	channels := l.registerKeys([]int{1, 1})
	close(sourceRelease)
	select {
	case <-cloneStarted:
	case <-ctx.Done():
		t.Fatal("cloner did not start")
	}
	done := make(chan error, 1)
	go func() { _, err := l.LoadAndStore(ctx, 2); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-ctx.Done():
		t.Error("cloning key 1 blocks loading unrelated key 2")
	}
	finishClone()
	if _, err := l.awaitChannels(ctx, channels); err != nil {
		t.Error(err)
	}
}

// Do not run in parallel: other loader tests also create loading goroutines.
func TestCanceledWaitersLeaveNoDrainers(t *testing.T) {
	for _, multi := range []bool{false, true} {
		name := "Single"
		if multi {
			name = "Multi"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const count = 32
				baseline := countLoaderGoroutines()
				release := make(chan struct{})
				finishLoad := sync.OnceFunc(func() { close(release) })
				defer finishLoad()
				src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
					<-release
					return make([]*loadingcache.CacheEntry[int, int], len(keys)), nil
				})
				l := NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var returned [count]bool
				var errs [count]error
				for i := range count {
					go func() {
						if multi {
							_, errs[i] = l.LoadAndStoreMulti(ctx, []int{1, 1})
						} else {
							_, errs[i] = l.LoadAndStore(ctx, 1)
						}
						returned[i] = true
					}()
				}
				synctest.Wait()
				waiterCount := count
				if multi {
					waiterCount *= 2
				}
				l.mu.Lock()
				registered := len(l.waitlists[1])
				l.mu.Unlock()
				if registered != waiterCount {
					t.Fatalf("registered %d waiters, want %d", registered, waiterCount)
				}
				if got := countLoaderGoroutines(); got != baseline+count+1 {
					t.Fatalf("goroutine filter found %d goroutines, want %d registered callers and shared load", got, baseline+count+1)
				}
				cancel()
				synctest.Wait()
				for i := range count {
					if !returned[i] || !errors.Is(errs[i], context.Canceled) {
						t.Errorf("waiter %d: returned %t, error %v; want context.Canceled", i, returned[i], errs[i])
					}
				}
				if delta := countLoaderGoroutines() - baseline; delta != 1 {
					t.Errorf("%d canceled callers leave %d goroutines; only the shared load should remain", count, delta)
				}
				finishLoad()
				synctest.Wait()
				if got := countLoaderGoroutines(); got != baseline {
					t.Errorf("loader goroutines after source completion: got %d, want baseline %d", got, baseline)
				}
			})
		})
	}
}

func countLoaderGoroutines() int {
	buf := make([]byte, 64<<10)
	for {
		n := runtime.Stack(buf, true)
		if n == len(buf) {
			buf = make([]byte, 2*len(buf))
			continue
		}
		count := 0
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			// Include public methods and their closures: a drainer created by
			// LoadAndStore would not have a loadKeyAndStore frame.
			if strings.Contains(stack, "github.com/karupanerura/loading-cache/loader/singleflightloader.(*SingleFlightLoader[") {
				count++
			}
		}
		return count
	}
}
