package omcindex_test

import (
	"context"
	"testing"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

func BenchmarkGetSharedContext(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
		return map[int][]int{1: {2}}, nil
	}))
	if err := idx.Refresh(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			keys, err := idx.Get(ctx, 1)
			if err != nil || len(keys) != 1 || keys[0] != 2 {
				b.Errorf("Get: %v, %v", keys, err)
				return
			}
		}
	})
}

// Measure initialized Get with context creation and cancellation per iteration,
// including their allocations. The shared-context benchmark excludes that setup.
func BenchmarkGetFreshContext(b *testing.B) {
	idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
		return map[int][]int{1: {2}}, nil
	}))
	if err := idx.Refresh(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, cancel := context.WithCancel(context.Background())
			keys, err := idx.Get(ctx, 1)
			cancel()
			if err != nil || len(keys) != 1 || keys[0] != 2 {
				b.Errorf("Get: %v, %v", keys, err)
				return
			}
		}
	})
}
