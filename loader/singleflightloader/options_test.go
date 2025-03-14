package singleflightloader

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

func TestWithCloner(t *testing.T) {
	// Define a custom cloner for test
	customCloner := loadingcache.ValueClonerFunc[string](func(v string) string {
		return v + "_cloned"
	})

	tests := []struct {
		name          string
		option        Option[int, string]
		originalValue string
		wantValue     string
	}{
		{
			name:          "default cloner (no option)",
			option:        nil,
			originalValue: "test",
			wantValue:     "test", // NopValueCloner returns the same value
		},
		{
			name:          "custom cloner",
			option:        WithCloner[int, string](customCloner),
			originalValue: "test",
			wantValue:     "test_cloned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new PureLoader with mock storage and source
			mockStorage := &storage.FunctionsStorage[int, string]{}
			mockSource := &source.FunctionsSource[int, string]{}

			var loader *SingleFlightLoader[int, string]
			if tt.option == nil {
				loader = NewSingleFlightLoader(mockStorage, mockSource)
			} else {
				loader = NewSingleFlightLoader(mockStorage, mockSource, tt.option)
			}

			// Verify that the cloner is properly set by cloning a value
			gotValue := loader.cloner.CloneValue(tt.originalValue)

			if diff := cmp.Diff(tt.wantValue, gotValue); diff != "" {
				t.Errorf("Cloned value mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestOptionApplicationOrder tests that multiple options are applied in the correct order
func TestOptionApplicationOrder(t *testing.T) {
	// Create a test value cloner
	type testStruct struct {
		Value string
	}

	// Define two custom cloners for testing application order
	firstCloner := loadingcache.ValueClonerFunc[testStruct](func(v testStruct) testStruct {
		return testStruct{Value: v.Value + "_first"}
	})

	secondCloner := loadingcache.ValueClonerFunc[testStruct](func(v testStruct) testStruct {
		return testStruct{Value: v.Value + "_second"}
	})

	// Create a new PureLoader with mock storage and source
	mockStorage := &storage.FunctionsStorage[int, testStruct]{}
	mockSource := &source.FunctionsSource[int, testStruct]{}

	// Apply options in sequence - the last one should override previous ones
	loader := NewSingleFlightLoader(mockStorage, mockSource,
		WithCloner[int, testStruct](firstCloner),
		WithCloner[int, testStruct](secondCloner),
	)

	// Test value to clone
	original := testStruct{Value: "test"}

	// The second cloner should be the one that's used
	expected := testStruct{Value: "test_second"}
	actual := loader.cloner.CloneValue(original)

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf("Option application order test failed (-want +got):\n%s", diff)
	}
}
