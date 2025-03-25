package iterutil_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/internal/iterutil"
)

func TestIntersection(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		inputs [][]uint8
		want   []uint8
	}{
		{
			name:   "empty",
			inputs: [][]uint8{},
			want:   nil,
		},
		{
			name:   "single empty slice",
			inputs: [][]uint8{{}},
			want:   nil,
		},
		{
			name:   "multiple empty slices",
			inputs: [][]uint8{{}, {}, {}},
			want:   nil,
		},
		{
			name:   "single non-empty slice",
			inputs: [][]uint8{{1, 2, 3}},
			want:   []uint8{1, 2, 3},
		},
		{
			name:   "two slices with intersection",
			inputs: [][]uint8{{1, 2, 3}, {2, 3, 4}},
			want:   []uint8{2, 3},
		},
		{
			name:   "two slices with no intersection",
			inputs: [][]uint8{{1, 2}, {3, 4}},
			want:   nil,
		},
		{
			name:   "multiple slices with common element",
			inputs: [][]uint8{{1, 2, 3}, {2, 3, 4}, {3, 5, 6}},
			want:   []uint8{3},
		},
		{
			name:   "multiple slices with no common element",
			inputs: [][]uint8{{1, 2}, {3, 4}, {5, 6}},
			want:   nil,
		},
		{
			name:   "slices with duplicate elements",
			inputs: [][]uint8{{1, 2, 2}, {2, 2, 3}},
			want:   []uint8{2},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create slices.Seq iterators for each input slice
			iters := make([]iter.Seq[uint8], 0, len(tt.inputs))
			for _, input := range tt.inputs {
				iters = append(iters, slices.Values(input))
			}

			// Find intersection of the iterators and collect the results
			got := slices.Collect(iterutil.Intersection(iters...))

			// Sort to ensure consistent comparison order when duplicates are present
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIntersection_Break(t *testing.T) {
	t.Parallel()

	counter := uint8(0)
	seq1 := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(0); i < 100; i++ {
			if !yield(i) {
				return
			}
			counter++
		}
	})

	seq2 := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(0); i < 100; i++ {
			if !yield(i) {
				return
			}
			counter++
		}
	})

	for v := range iterutil.Intersection(seq1, seq2) {
		if v == 20 {
			break
		}
	}

	if counter != 120 {
		t.Errorf("unexpected counter value: %d", counter)
	}
}

func TestUniq(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		input []uint8
		want  []uint8
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name:  "no duplicates",
			input: []uint8{1, 2, 3},
			want:  []uint8{1, 2, 3},
		},
		{
			name:  "with duplicates",
			input: []uint8{1, 1, 2, 2, 3},
			want:  []uint8{1, 2, 3},
		},
		{
			name:  "all duplicates",
			input: []uint8{1, 1, 1, 1},
			want:  []uint8{1},
		},
		{
			name:  "single element",
			input: []uint8{1},
			want:  []uint8{1},
		},
		{
			name:  "duplicates not adjacent",
			input: []uint8{1, 2, 1, 3, 2, 4},
			want:  []uint8{1, 2, 3, 4},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create iterator and apply Uniq
			got := slices.Collect(iterutil.Uniq(slices.Values(tt.input)))

			// Sort results to ensure consistent comparison order
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUniq_Break(t *testing.T) {
	t.Parallel()

	counter := uint8(0)
	seq := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(0); i < 100; i++ {
			for j := uint8(0); j < 2; j++ {
				if !yield(i) {
					return
				}
				counter++
			}
		}
	})

	for v := range iterutil.Uniq(seq) {
		if v == 10 {
			break
		}
	}

	if counter != 20 {
		t.Errorf("unexpected counter value: %d, should be exactly 20", counter)
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		input []uint8
		want  []uint16
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name:  "non-empty",
			input: []uint8{1, 2, 3},
			want:  []uint16{2, 4, 6},
		},
		{
			name:  "single element",
			input: []uint8{5},
			want:  []uint16{10},
		},
		{
			name:  "with duplicates",
			input: []uint8{1, 1, 2, 2, 3},
			want:  []uint16{2, 2, 4, 4, 6},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create iterator and apply Map to double each value
			doubleFunc := func(v uint8) uint16 {
				return uint16(v) * 2
			}
			seq := slices.Values(tt.input)
			got := slices.Collect(iterutil.Map(seq, doubleFunc))

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMap_Break(t *testing.T) {
	t.Parallel()

	counter := uint8(0)
	seq := iter.Seq[uint8](func(yield func(uint8) bool) {
		for {
			if !yield(counter) {
				return
			}
			counter++
		}
	})

	doubleFunc := func(v uint8) uint16 {
		return uint16(v) * 2
	}

	for v := range iterutil.Map(seq, doubleFunc) {
		if v == 40 { // This is double of 20
			break
		}
	}

	if counter != 20 {
		t.Errorf("unexpected counter value: %d, should be exactly 20", counter)
	}
}

func TestUnion(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		inputs [][]uint8
		want   []uint8
	}{
		{
			name:   "empty",
			inputs: [][]uint8{},
			want:   nil,
		},
		{
			name:   "single empty slice",
			inputs: [][]uint8{{}},
			want:   nil,
		},
		{
			name:   "multiple empty slices",
			inputs: [][]uint8{{}, {}, {}},
			want:   nil,
		},
		{
			name:   "single non-empty slice",
			inputs: [][]uint8{{1, 2, 3}},
			want:   []uint8{1, 2, 3},
		},
		{
			name:   "two slices with intersection",
			inputs: [][]uint8{{1, 2, 3}, {2, 3, 4}},
			want:   []uint8{1, 2, 3, 4},
		},
		{
			name:   "two slices with no intersection",
			inputs: [][]uint8{{1, 2}, {3, 4}},
			want:   []uint8{1, 2, 3, 4},
		},
		{
			name:   "multiple slices with common element",
			inputs: [][]uint8{{1, 2, 3}, {2, 3, 4}, {3, 5, 6}},
			want:   []uint8{1, 2, 3, 4, 5, 6},
		},
		{
			name:   "slices with duplicate elements",
			inputs: [][]uint8{{1, 2, 2}, {2, 2, 3}},
			want:   []uint8{1, 2, 3},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create slices.Seq iterators for each input slice
			iters := make([]iter.Seq[uint8], 0, len(tt.inputs))
			for _, input := range tt.inputs {
				iters = append(iters, slices.Values(input))
			}

			// Find union of the iterators and collect the results
			got := slices.Collect(iterutil.Union(iters...))

			// Sort to ensure consistent comparison order
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnion_Break(t *testing.T) {
	t.Parallel()

	counter := uint8(0)
	seq1 := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(0); i < 50; i++ {
			if !yield(i) {
				return
			}
			counter++
		}
	})

	seq2 := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(25); i < 75; i++ {
			if !yield(i) {
				return
			}
			counter++
		}
	})

	for v := range iterutil.Union(seq1, seq2) {
		if v == 20 {
			break
		}
	}

	// Should have processed elements 0-20 from seq1 and none from seq2
	if counter != 20 {
		t.Errorf("unexpected counter value: %d, should be exactly 20", counter)
	}
}

func TestFlatMap(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		input []uint8
		want  []uint16
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name:  "single element",
			input: []uint8{1},
			want:  []uint16{1, 2},
		},
		{
			name:  "multiple elements",
			input: []uint8{1, 2, 3},
			want:  []uint16{1, 2, 2, 4, 3, 6},
		},
		{
			name:  "with empty mapping",
			input: []uint8{0, 1, 2},
			want:  []uint16{1, 2, 2, 4}, // 0 produces empty sequence
		},
		{
			name:  "with duplicate elements",
			input: []uint8{1, 1, 2},
			want:  []uint16{1, 2, 1, 2, 2, 4},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create mapping function that produces a sequence for each input value
			// For 0, return empty seq, otherwise return [v, v*2]
			mapFunc := func(v uint8) iter.Seq[uint16] {
				if v == 0 {
					return iter.Seq[uint16](func(yield func(uint16) bool) {})
				}
				return iter.Seq[uint16](func(yield func(uint16) bool) {
					if !yield(uint16(v)) {
						return
					}
					if !yield(uint16(v) * 2) {
						return
					}
				})
			}

			// Apply FlatMap to input
			seq := slices.Values(tt.input)
			got := slices.Collect(iterutil.FlatMap(seq, mapFunc))

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFlatMap_Break(t *testing.T) {
	t.Parallel()

	// Tracks how many input elements were processed
	inputCounter := uint8(0)
	seq := iter.Seq[uint8](func(yield func(uint8) bool) {
		for i := uint8(0); i < 100; i++ {
			inputCounter++
			if !yield(i) {
				return
			}
		}
	})

	// Tracks how many output elements were produced
	outputCounter := uint16(0)
	mapFunc := func(v uint8) iter.Seq[uint16] {
		return iter.Seq[uint16](func(yield func(uint16) bool) {
			// For each input, generate 3 output elements
			for i := uint16(0); i < 3; i++ {
				outputCounter++
				val := uint16(v)*10 + i
				if !yield(val) {
					return
				}
			}
		})
	}

	// Break after seeing value 23
	for v := range iterutil.FlatMap(seq, mapFunc) {
		if v == 22 { // This is the first element from input 2 (20, 21, 22)
			break
		}
	}

	// Should process 3 full inputs (0, 1, 2) = 9 outputs
	// Should reach input 3 but only process part of its outputs
	if inputCounter != 3 {
		t.Errorf("unexpected input counter value: %d, should be exactly 3", inputCounter)
	}
	if outputCounter != 9 {
		t.Errorf("unexpected output counter value: %d, should be exactly 9", outputCounter)
	}
}
