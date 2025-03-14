package loadingcache_test

import (
	"testing"

	loadingcache "github.com/karupanerura/loading-cache"
)

// Test structs with different cloning behaviors
type TestClonerStruct struct {
	Value int
}

func (s *TestClonerStruct) Clone() *TestClonerStruct {
	return &TestClonerStruct{
		Value: s.Value,
	}
}

type TestDeepCopyerStruct struct {
	Value int
}

func (s *TestDeepCopyerStruct) DeepCopy() *TestDeepCopyerStruct {
	return &TestDeepCopyerStruct{
		Value: s.Value,
	}
}

func TestDefaultClonerWithCloneMethod(t *testing.T) {
	t.Parallel()

	// Test with pointer type that has Clone method
	cloner := loadingcache.DefaultValueCloner[*TestClonerStruct]()
	original := &TestClonerStruct{Value: 42}
	cloned := cloner.CloneValue(original)

	if original == cloned {
		t.Error("Expected different pointer, got same pointer")
	}
	if original.Value != cloned.Value {
		t.Errorf("Expected same value, got original=%d, cloned=%d", original.Value, cloned.Value)
	}

	// Modify original to verify deep copy
	original.Value = 100
	if cloned.Value != 42 {
		t.Errorf("Expected cloned value to remain unchanged, got %d", cloned.Value)
	}
}

func TestDefaultClonerWithDeepCopyMethod(t *testing.T) {
	t.Parallel()

	// Test with pointer type that has DeepCopy method
	cloner := loadingcache.DefaultValueCloner[*TestDeepCopyerStruct]()
	original := &TestDeepCopyerStruct{Value: 42}
	cloned := cloner.CloneValue(original)

	if original == cloned {
		t.Error("Expected different pointer, got same pointer")
	}
	if original.Value != cloned.Value {
		t.Errorf("Expected same value, got original=%d, cloned=%d", original.Value, cloned.Value)
	}

	// Modify original to verify deep copy
	original.Value = 100
	if cloned.Value != 42 {
		t.Errorf("Expected cloned value to remain unchanged, got %d", cloned.Value)
	}
}

func TestDefaultClonerWithNoSpecialMethod(t *testing.T) {
	t.Parallel()

	// Test with pointer type that has no Clone or DeepCopy method
	type SimpleStruct struct {
		Value int
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for type with no special methods, but did not panic")
		}
	}()
	loadingcache.DefaultValueCloner[*SimpleStruct]()
}

func TestDefaultClonerImplementation(t *testing.T) {
	t.Parallel()

	// Verify the correct interface implementation is chosen
	clonerStruct := loadingcache.DefaultValueCloner[*TestClonerStruct]()
	deepCopyerStruct := loadingcache.DefaultValueCloner[*TestDeepCopyerStruct]()
	stringCloner := loadingcache.DefaultValueCloner[string]()
	intCloner := loadingcache.DefaultValueCloner[int]()

	// Check if the cloner is ValueClonerFunc
	_, ok := clonerStruct.(loadingcache.ValueClonerFunc[*TestClonerStruct])
	if !ok {
		t.Error("Expected ValueClonerFunc for type with Clone method")
	}

	// Check if the deep copier is ValueClonerFunc
	_, ok = deepCopyerStruct.(loadingcache.ValueClonerFunc[*TestDeepCopyerStruct])
	if !ok {
		t.Error("Expected ValueClonerFunc for type with DeepCopy method")
	}

	// Check if string gets NopValueCloner
	_, ok = stringCloner.(loadingcache.NopValueCloner[string])
	if !ok {
		t.Error("Expected NopValueCloner for type with no special methods")
	}

	// Check if int gets NopValueCloner
	_, ok = intCloner.(loadingcache.NopValueCloner[int])
	if !ok {
		t.Error("Expected NopValueCloner for type with no special methods")
	}
}
