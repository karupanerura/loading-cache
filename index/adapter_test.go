package index_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index"
)

func TestFunctionIndexSource_GetAll(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source error")
	tests := []struct {
		name       string
		getAllFunc func(context.Context) (map[uint8][]uint8, error)
		wantResult map[uint8][]uint8
		wantErr    error
	}{
		{
			name: "successful get all",
			getAllFunc: func(context.Context) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{
					1: {10, 11},
					2: {20, 21},
				}, nil
			},
			wantResult: map[uint8][]uint8{
				1: {10, 11},
				2: {20, 21},
			},
			wantErr: nil,
		},
		{
			name: "error from source",
			getAllFunc: func(context.Context) (map[uint8][]uint8, error) {
				return nil, sourceErr
			},
			wantResult: nil,
			wantErr:    sourceErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			indexSource := index.FunctionIndexSource[uint8, uint8](
				tt.getAllFunc,
			)

			gotResult, gotErr := indexSource.GetAll(t.Context())
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantResult, gotResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFunctionIndex_Get(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source error")
	tests := []struct {
		name       string
		getFunc    func(context.Context, uint8) ([]uint8, error)
		key        uint8
		wantResult []uint8
		wantErr    error
	}{
		{
			name: "successful get",
			getFunc: func(ctx context.Context, key uint8) ([]uint8, error) {
				if key == 1 {
					return []uint8{10, 11}, nil
				}
				return nil, nil
			},
			key:        1,
			wantResult: []uint8{10, 11},
			wantErr:    nil,
		},
		{
			name: "key not found",
			getFunc: func(ctx context.Context, key uint8) ([]uint8, error) {
				return nil, nil
			},
			key:        2,
			wantResult: nil,
			wantErr:    nil,
		},
		{
			name: "error from source",
			getFunc: func(ctx context.Context, key uint8) ([]uint8, error) {
				return nil, sourceErr
			},
			key:        3,
			wantResult: nil,
			wantErr:    sourceErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			index := &index.FunctionsIndex[uint8, uint8]{
				GetFunc: tt.getFunc,
			}

			gotResult, gotErr := index.Get(context.Background(), tt.key)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantResult, gotResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFunctionIndex_GetMulti(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source error")
	tests := []struct {
		name         string
		getMultiFunc func(context.Context, []uint8) (map[uint8][]uint8, error)
		keys         []uint8
		wantResult   map[uint8][]uint8
		wantErr      error
	}{
		{
			name: "successful get multi",
			getMultiFunc: func(ctx context.Context, keys []uint8) (map[uint8][]uint8, error) {
				result := make(map[uint8][]uint8)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint8{10, 11}
					} else if key == 2 {
						result[key] = []uint8{20, 21}
					}
				}
				return result, nil
			},
			keys: []uint8{1, 2},
			wantResult: map[uint8][]uint8{
				1: {10, 11},
				2: {20, 21},
			},
			wantErr: nil,
		},
		{
			name: "partial keys not found",
			getMultiFunc: func(ctx context.Context, keys []uint8) (map[uint8][]uint8, error) {
				result := make(map[uint8][]uint8)
				for _, key := range keys {
					if key == 1 {
						result[key] = []uint8{10, 11}
					}
				}
				return result, nil
			},
			keys: []uint8{1, 2},
			wantResult: map[uint8][]uint8{
				1: {10, 11},
			},
			wantErr: nil,
		},
		{
			name: "any keys not found",
			getMultiFunc: func(ctx context.Context, keys []uint8) (map[uint8][]uint8, error) {
				return map[uint8][]uint8{}, nil
			},
			keys:       []uint8{1, 2},
			wantResult: map[uint8][]uint8{},
			wantErr:    nil,
		},
		{
			name: "error from source",
			getMultiFunc: func(ctx context.Context, keys []uint8) (map[uint8][]uint8, error) {
				return nil, sourceErr
			},
			keys:       []uint8{1, 2},
			wantResult: nil,
			wantErr:    sourceErr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			index := &index.FunctionsIndex[uint8, uint8]{
				GetMultiFunc: tt.getMultiFunc,
			}

			gotResult, gotErr := index.GetMulti(context.Background(), tt.keys)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantResult, gotResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
