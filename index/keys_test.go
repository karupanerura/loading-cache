package index_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index"
)

func TestZipKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  []int
		right []string
		want  []index.Keys[int, string]
	}{
		{
			name:  "equal length slices",
			left:  []int{1, 2, 3},
			right: []string{"a", "b", "c"},
			want: []index.Keys[int, string]{
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 1}, Right: index.MaybeKey[string]{Key: "a"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 2}, Right: index.MaybeKey[string]{Key: "b"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 3}, Right: index.MaybeKey[string]{Key: "c"}},
			},
		},
		{
			name:  "left slice longer than right",
			left:  []int{1, 2, 3, 4},
			right: []string{"a", "b"},
			want: []index.Keys[int, string]{
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 1}, Right: index.MaybeKey[string]{Key: "a"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 2}, Right: index.MaybeKey[string]{Key: "b"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 3}, Right: index.MaybeKey[string]{Empty: true}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 4}, Right: index.MaybeKey[string]{Empty: true}},
			},
		},
		{
			name:  "right slice longer than left",
			left:  []int{1, 2},
			right: []string{"a", "b", "c", "d"},
			want: []index.Keys[int, string]{
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 1}, Right: index.MaybeKey[string]{Key: "a"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 2}, Right: index.MaybeKey[string]{Key: "b"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "c"}},
				index.Keys[int, string]{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "d"}},
			},
		},
		{
			name:  "single element slices",
			left:  []int{1},
			right: []string{"a"},
			want: []index.Keys[int, string]{
				index.NewKeys(1, "a"),
			},
		},
		{
			name:  "empty right slice",
			left:  []int{1},
			right: []string{},
			want: []index.Keys[int, string]{
				index.Keys[int, string]{Left: index.MaybeKey[int]{Key: 1}, Right: index.MaybeKey[string]{Empty: true}},
			},
		},
		{
			name:  "empty left slice",
			left:  []int{},
			right: []string{"a"},
			want: []index.Keys[int, string]{
				index.Keys[int, string]{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "a"}},
			},
		},
		{
			name:  "empty both slice",
			left:  []int{},
			right: []string{},
			want:  []index.Keys[int, string]{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := index.ZipKeys(tt.left, tt.right)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLeftKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		left int
		want index.Keys[int, string]
	}{
		{
			name: "basic case",
			left: 42,
			want: index.Keys[int, string]{
				Left:  index.MaybeKey[int]{Key: 42},
				Right: index.MaybeKey[string]{Empty: true},
			},
		},
		{
			name: "zero value",
			left: 0,
			want: index.Keys[int, string]{
				Left:  index.MaybeKey[int]{Key: 0},
				Right: index.MaybeKey[string]{Empty: true},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := index.LeftKey[int, string](tt.left)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRightKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		right string
		want  index.Keys[int, string]
	}{
		{
			name:  "basic case",
			right: "test",
			want: index.Keys[int, string]{
				Left:  index.MaybeKey[int]{Empty: true},
				Right: index.MaybeKey[string]{Key: "test"},
			},
		},
		{
			name:  "empty string",
			right: "",
			want: index.Keys[int, string]{
				Left:  index.MaybeKey[int]{Empty: true},
				Right: index.MaybeKey[string]{Key: ""},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := index.RightKey[int, string](tt.right)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLeftKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		left []int
		want []index.Keys[int, string]
	}{
		{
			name: "multiple elements",
			left: []int{1, 2, 3},
			want: []index.Keys[int, string]{
				{Left: index.MaybeKey[int]{Key: 1}, Right: index.MaybeKey[string]{Empty: true}},
				{Left: index.MaybeKey[int]{Key: 2}, Right: index.MaybeKey[string]{Empty: true}},
				{Left: index.MaybeKey[int]{Key: 3}, Right: index.MaybeKey[string]{Empty: true}},
			},
		},
		{
			name: "single element",
			left: []int{42},
			want: []index.Keys[int, string]{
				{Left: index.MaybeKey[int]{Key: 42}, Right: index.MaybeKey[string]{Empty: true}},
			},
		},
		{
			name: "empty slice",
			left: []int{},
			want: []index.Keys[int, string]{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := index.LeftKeys[int, string](tt.left)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRightKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		right []string
		want  []index.Keys[int, string]
	}{
		{
			name:  "multiple elements",
			right: []string{"a", "b", "c"},
			want: []index.Keys[int, string]{
				{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "a"}},
				{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "b"}},
				{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "c"}},
			},
		},
		{
			name:  "single element",
			right: []string{"test"},
			want: []index.Keys[int, string]{
				{Left: index.MaybeKey[int]{Empty: true}, Right: index.MaybeKey[string]{Key: "test"}},
			},
		},
		{
			name:  "empty slice",
			right: []string{},
			want:  []index.Keys[int, string]{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := index.RightKeys[int, string](tt.right)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaybeKey_Iter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maybeKey index.MaybeKey[int]
		want     []int
	}{
		{
			name:     "non-empty key",
			maybeKey: index.MaybeKey[int]{Key: 42, Empty: false},
			want:     []int{42},
		},
		{
			name:     "empty key",
			maybeKey: index.MaybeKey[int]{Key: 0, Empty: true},
			want:     nil,
		},
		{
			name:     "zero value non-empty key",
			maybeKey: index.MaybeKey[int]{Key: 0, Empty: false},
			want:     []int{0},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Collect the iterator results
			var got []int
			for v := range tt.maybeKey.Iter() {
				got = append(got, v)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaybeKey_Iter_Break(t *testing.T) {
	t.Parallel()

	maybeKey := index.MaybeKey[bool]{Key: true}
	for range maybeKey.Iter() {
		break
	}
	// No panic is expected
}
