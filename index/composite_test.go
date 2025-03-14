package index_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index"
)

func TestOrIndex_Get(t *testing.T) {
	t.Parallel()

	leftErr := errors.New("left error")
	rightErr := errors.New("right error")
	tests := []struct {
		name       string
		leftFunc   func(context.Context, int8) ([]uint16, error)
		rightFunc  func(context.Context, uint8) ([]uint16, error)
		key        index.Keys[int8, uint8]
		wantResult []uint16
		wantErr    error
	}{
		{
			name: "successful get with non-overlapping results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				if key == 1 {
					return []uint16{10, 11}, nil
				}
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				if key == 2 {
					return []uint16{20, 21}, nil
				}
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: []uint16{10, 11, 20, 21},
			wantErr:    nil,
		},
		{
			name: "successful get with overlapping results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				if key == 1 {
					return []uint16{10, 11, 12}, nil
				}
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				if key == 2 {
					return []uint16{11, 12, 13}, nil
				}
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: []uint16{10, 11, 12, 13},
			wantErr:    nil,
		},
		{
			name: "successful get with empty left results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return []uint16{20, 21}, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: []uint16{20, 21},
			wantErr:    nil,
		},
		{
			name: "successful get with empty right results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return []uint16{10, 11}, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: []uint16{10, 11},
			wantErr:    nil,
		},
		{
			name: "successful get with empty results from both",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "error from left index",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, leftErr
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return []uint16{20, 21}, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    leftErr,
		},
		{
			name: "error from right index",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return []uint16{10, 11}, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, rightErr
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    rightErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leftIndex := &index.FunctionsIndex[int8, uint16]{
				GetFunc: tt.leftFunc,
			}
			rightIndex := &index.FunctionsIndex[uint8, uint16]{
				GetFunc: tt.rightFunc,
			}

			orIndex := &index.OrIndex[int8, uint8, uint16]{
				Left:  leftIndex,
				Right: rightIndex,
			}

			gotResult, gotErr := orIndex.Get(context.Background(), tt.key)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			// Sort results to ensure consistent comparison
			slices.Sort(gotResult)
			expected := slices.Clone(tt.wantResult)
			slices.Sort(expected)

			if diff := cmp.Diff(expected, gotResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOrIndex_GetMulti(t *testing.T) {
	t.Parallel()

	leftErr := errors.New("left error")
	rightErr := errors.New("right error")
	tests := []struct {
		name          string
		leftGetMulti  func(context.Context, []int8) (map[int8][]uint16, error)
		rightGetMulti func(context.Context, []uint8) (map[uint8][]uint16, error)
		keys          []index.Keys[int8, uint8]
		wantResult    map[index.Keys[int8, uint8]][]uint16
		wantErr       error
	}{
		{
			name: "successful get multi with non-overlapping results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					} else if key == 3 {
						result[key] = []uint16{30, 31}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{20, 21}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: {10, 11, 20, 21},
				{Left: 3, Right: 4}: {30, 31, 40, 41},
			},
			wantErr: nil,
		},
		{
			name: "successful get multi with overlapping results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11, 12}
					} else if key == 3 {
						result[key] = []uint16{30, 31, 32}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{11, 12, 13}
					} else if key == 4 {
						result[key] = []uint16{31, 32, 33}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: {10, 11, 12, 13},
				{Left: 3, Right: 4}: {30, 31, 32, 33},
			},
			wantErr: nil,
		},
		{
			name: "successful get multi with some empty results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					}
					// key 3 returns empty result
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{20, 21}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: {10, 11, 20, 21},
				{Left: 3, Right: 4}: {40, 41},
			},
			wantErr: nil,
		},
		{
			name: "error from left index",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				return nil, leftErr
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{20, 21}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: nil,
			wantErr:    leftErr,
		},
		{
			name: "error from right index",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					} else if key == 3 {
						result[key] = []uint16{30, 31}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				return nil, rightErr
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: nil,
			wantErr:    rightErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leftIndex := &index.FunctionsIndex[int8, uint16]{
				GetMultiFunc: tt.leftGetMulti,
			}
			rightIndex := &index.FunctionsIndex[uint8, uint16]{
				GetMultiFunc: tt.rightGetMulti,
			}

			orIndex := &index.OrIndex[int8, uint8, uint16]{
				Left:  leftIndex,
				Right: rightIndex,
			}

			gotResult, gotErr := orIndex.GetMulti(context.Background(), tt.keys)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if gotErr != nil {
				return
			}

			if diff := cmp.Diff(tt.wantResult, gotResult, cmp.Comparer(func(lhs, rhs []uint16) bool {
				if lhs == nil {
					return rhs == nil
				} else if rhs == nil {
					return lhs == nil
				}
				lhs = slices.Clone(lhs)
				rhs = slices.Clone(rhs)
				slices.Sort(lhs)
				slices.Sort(rhs)
				return slices.Equal(lhs, rhs)
			})); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAndIndex_Get(t *testing.T) {
	t.Parallel()

	leftErr := errors.New("left error")
	rightErr := errors.New("right error")
	tests := []struct {
		name       string
		leftFunc   func(context.Context, int8) ([]uint16, error)
		rightFunc  func(context.Context, uint8) ([]uint16, error)
		key        index.Keys[int8, uint8]
		wantResult []uint16
		wantErr    error
	}{
		{
			name: "successful get with overlapping results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				if key == 1 {
					return []uint16{10, 11, 12}, nil
				}
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				if key == 2 {
					return []uint16{11, 12, 13}, nil
				}
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: []uint16{11, 12},
			wantErr:    nil,
		},
		{
			name: "successful get with no overlapping results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				if key == 1 {
					return []uint16{10, 11}, nil
				}
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				if key == 2 {
					return []uint16{20, 21}, nil
				}
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "successful get with empty left results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return []uint16{20, 21}, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "successful get with empty right results",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return []uint16{10, 11}, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "successful get with empty results from both",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "error from left index",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return nil, leftErr
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return []uint16{20, 21}, nil
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    leftErr,
		},
		{
			name: "error from right index",
			leftFunc: func(ctx context.Context, key int8) ([]uint16, error) {
				return []uint16{10, 11}, nil
			},
			rightFunc: func(ctx context.Context, key uint8) ([]uint16, error) {
				return nil, rightErr
			},
			key:        index.Keys[int8, uint8]{Left: 1, Right: 2},
			wantResult: nil,
			wantErr:    rightErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leftIndex := &index.FunctionsIndex[int8, uint16]{
				GetFunc: tt.leftFunc,
			}
			rightIndex := &index.FunctionsIndex[uint8, uint16]{
				GetFunc: tt.rightFunc,
			}

			andIndex := &index.AndIndex[int8, uint8, uint16]{
				Left:  leftIndex,
				Right: rightIndex,
			}

			gotResult, gotErr := andIndex.Get(context.Background(), tt.key)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			// Sort results to ensure consistent comparison
			slices.Sort(gotResult)
			expected := slices.Clone(tt.wantResult)
			slices.Sort(expected)

			if diff := cmp.Diff(expected, gotResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAndIndex_GetMulti(t *testing.T) {
	t.Parallel()

	leftErr := errors.New("left error")
	rightErr := errors.New("right error")
	tests := []struct {
		name          string
		leftGetMulti  func(context.Context, []int8) (map[int8][]uint16, error)
		rightGetMulti func(context.Context, []uint8) (map[uint8][]uint16, error)
		keys          []index.Keys[int8, uint8]
		wantResult    map[index.Keys[int8, uint8]][]uint16
		wantErr       error
	}{
		{
			name: "successful get multi with overlapping results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11, 12}
					} else if key == 3 {
						result[key] = []uint16{30, 31, 32}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{11, 12, 13}
					} else if key == 4 {
						result[key] = []uint16{31, 32, 33}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: {11, 12},
				{Left: 3, Right: 4}: {31, 32},
			},
			wantErr: nil,
		},
		{
			name: "successful get multi with no overlapping results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					} else if key == 3 {
						result[key] = []uint16{30, 31}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{20, 21}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: nil,
				{Left: 3, Right: 4}: nil,
			},
			wantErr: nil,
		},
		{
			name: "successful get multi with some empty results",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					}
					// key 3 returns empty result
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{10, 11}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: map[index.Keys[int8, uint8]][]uint16{
				{Left: 1, Right: 2}: {10, 11},
				{Left: 3, Right: 4}: nil,
			},
			wantErr: nil,
		},
		{
			name: "error from left index",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				return nil, leftErr
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				result := make(map[uint8][]uint16)
				for _, key := range keys {
					if key == 2 {
						result[key] = []uint16{20, 21}
					} else if key == 4 {
						result[key] = []uint16{40, 41}
					}
				}
				return result, nil
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: nil,
			wantErr:    leftErr,
		},
		{
			name: "error from right index",
			leftGetMulti: func(ctx context.Context, keys []int8) (map[int8][]uint16, error) {
				result := make(map[int8][]uint16)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint16{10, 11}
					} else if key == 3 {
						result[key] = []uint16{30, 31}
					}
				}
				return result, nil
			},
			rightGetMulti: func(ctx context.Context, keys []uint8) (map[uint8][]uint16, error) {
				return nil, rightErr
			},
			keys: []index.Keys[int8, uint8]{
				{Left: 1, Right: 2},
				{Left: 3, Right: 4},
			},
			wantResult: nil,
			wantErr:    rightErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leftIndex := &index.FunctionsIndex[int8, uint16]{
				GetMultiFunc: tt.leftGetMulti,
			}
			rightIndex := &index.FunctionsIndex[uint8, uint16]{
				GetMultiFunc: tt.rightGetMulti,
			}

			andIndex := &index.AndIndex[int8, uint8, uint16]{
				Left:  leftIndex,
				Right: rightIndex,
			}

			gotResult, gotErr := andIndex.GetMulti(context.Background(), tt.keys)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if gotErr != nil {
				return
			}

			if diff := cmp.Diff(tt.wantResult, gotResult, cmp.Comparer(func(lhs, rhs []uint16) bool {
				if lhs == nil {
					return rhs == nil
				} else if rhs == nil {
					return lhs == nil
				}
				lhs = slices.Clone(lhs)
				rhs = slices.Clone(rhs)
				slices.Sort(lhs)
				slices.Sort(rhs)
				return slices.Equal(lhs, rhs)
			})); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
