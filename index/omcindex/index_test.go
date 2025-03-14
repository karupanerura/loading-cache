package omcindex_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

type testCase struct {
	name                   string
	sourceData             map[uint8][]uint8
	sourceErr              error
	inputKeys              []uint8
	singleKey              uint8
	expectedData           map[uint8][]uint8
	expectedSlice          []uint8
	expectedErr            error
	expectedBackgroundErrs []error
}

func TestOnMemoryIndex_RefreshError(t *testing.T) {
	t.Parallel()

	// Create mock source
	sourceErr := errors.New("source error")
	source := index.FunctionIndexSource[uint8, uint8](
		func(ctx context.Context) (map[uint8][]uint8, error) {
			return nil, sourceErr
		},
	)

	// Create and initialize index
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)
	if err := idx.Refresh(t.Context()); !errors.Is(err, sourceErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOnMemoryIndex_Get(t *testing.T) {
	t.Parallel()

	tests := []testCase{
		{
			name: "successful get with existing key",
			sourceData: map[uint8][]uint8{
				1: {10, 11},
				2: {20, 21},
			},
			singleKey:              1,
			expectedSlice:          []uint8{10, 11},
			expectedErr:            nil,
			expectedBackgroundErrs: nil,
		},
		{
			name: "key not found",
			sourceData: map[uint8][]uint8{
				1: {10, 11},
			},
			singleKey:              2,
			expectedSlice:          nil,
			expectedErr:            nil,
			expectedBackgroundErrs: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create mock source
			source := index.FunctionIndexSource[uint8, uint8](
				func(ctx context.Context) (map[uint8][]uint8, error) {
					return tt.sourceData, tt.sourceErr
				},
			)

			// Create and initialize index
			idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Fatalf("failed to initialize index: %v", err)
			}

			// Test Get method
			result, err := idx.Get(t.Context(), tt.singleKey)
			if tt.expectedErr == nil && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.expectedErr != nil && (err == nil || !errors.Is(err, tt.expectedErr)) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}

			if diff := cmp.Diff(tt.expectedSlice, result); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOnMemoryIndex_Get_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run("WaitForUpdateIndex", func(t *testing.T) {
		t.Parallel()

		// Create mock source
		source := index.FunctionIndexSource[uint8, uint8](
			func(ctx context.Context) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{
					1: {10, 11},
					2: {20, 21},
				}, nil
			},
		)

		// Create and initialize index
		idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

		// Update index in background
		go func() {
			time.Sleep(100 * time.Millisecond)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Errorf("failed to initialize index: %v", err)
				return
			}
		}()

		// Test Get method
		result, err := idx.Get(t.Context(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if diff := cmp.Diff([]uint8{10, 11}, result); diff != "" {
			t.Errorf("unexpected result (-want +got):\n%s", diff)
		}
	})

	t.Run("TimeoutToWaitRefresh", func(t *testing.T) {
		t.Parallel()

		// Create mock source
		source := index.FunctionIndexSource[uint8, uint8](
			func(ctx context.Context) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{
					1: {10, 11},
					2: {20, 21},
				}, nil
			},
		)

		// Create and initialize index
		idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

		// Update index in background
		go func() {
			time.Sleep(1 * time.Second)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Errorf("failed to initialize index: %v", err)
				return
			}
		}()

		// Test Get method
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		_, err := idx.Get(ctx, 1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("unexpected error: %v (expected: context deadline exceeded)", err)
		}
	})
}

func TestOnMemoryIndex_GetMulti(t *testing.T) {
	t.Parallel()

	tests := []testCase{
		{
			name: "successful get multi with all keys",
			sourceData: map[uint8][]uint8{
				1: {10, 11},
				2: {20, 21},
				3: {30, 31},
			},
			inputKeys: []uint8{1, 2},
			expectedData: map[uint8][]uint8{
				1: {10, 11},
				2: {20, 21},
			},
			expectedErr:            nil,
			expectedBackgroundErrs: nil,
		},
		{
			name: "partial key match",
			sourceData: map[uint8][]uint8{
				1: {10, 11},
				3: {30, 31},
			},
			inputKeys: []uint8{1, 2, 3},
			expectedData: map[uint8][]uint8{
				1: {10, 11},
				3: {30, 31},
			},
			expectedErr:            nil,
			expectedBackgroundErrs: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create mock source
			source := index.FunctionIndexSource[uint8, uint8](
				func(ctx context.Context) (map[uint8][]uint8, error) {
					return tt.sourceData, tt.sourceErr
				},
			)

			// Create and initialize index
			idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Fatalf("failed to initialize index: %v", err)
			}

			// Test GetMulti method
			result, err := idx.GetMulti(t.Context(), tt.inputKeys)

			// Check results
			if tt.expectedErr == nil && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.expectedErr != nil && (err == nil || !errors.Is(err, tt.expectedErr)) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}

			if diff := cmp.Diff(tt.expectedData, result); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOnMemoryInde_GetMulti_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run("WaitForUpdateIndex", func(t *testing.T) {
		t.Parallel()

		// Create mock source
		source := index.FunctionIndexSource[uint8, uint8](
			func(ctx context.Context) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{
					1: {10, 11},
					2: {20, 21},
					3: {30, 31},
				}, nil
			},
		)

		// Create and initialize index
		idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

		// Update index in background
		go func() {
			time.Sleep(100 * time.Millisecond)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Errorf("failed to initialize index: %v", err)
				return
			}
		}()

		// Test GetMulti method
		results, err := idx.GetMulti(t.Context(), []uint8{1, 3, 4})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if diff := cmp.Diff(map[uint8][]uint8{1: {10, 11}, 3: {30, 31}}, results); diff != "" {
			t.Errorf("unexpected result (-want +got):\n%s", diff)
		}
	})

	t.Run("TimeoutToWaitRefresh", func(t *testing.T) {
		t.Parallel()

		// Create mock source
		source := index.FunctionIndexSource[uint8, uint8](
			func(ctx context.Context) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{
					1: {10, 11},
					2: {20, 21},
				}, nil
			},
		)

		// Create and initialize index
		idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

		// Update index in background
		go func() {
			time.Sleep(1 * time.Second)
			if err := idx.Refresh(t.Context()); err != nil {
				t.Errorf("failed to initialize index: %v", err)
				return
			}
		}()

		// Test Get method
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		_, err := idx.GetMulti(ctx, []uint8{1, 3, 4})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("unexpected error: %v (expected: context deadline exceeded)", err)
		}
	})
}
