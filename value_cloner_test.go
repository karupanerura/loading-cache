package loadingcache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index/omcindex"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
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

// Shape is an interface type that declares Clone() Shape.
type Shape interface {
	Clone() Shape
	Width() int
	SetWidth(int)
}

type rect struct {
	width  int
	clones *int
}

func (r *rect) Clone() Shape {
	if r == nil {
		return (*rect)(nil)
	}
	if r.clones != nil {
		*r.clones++
	}
	return &rect{width: r.width, clones: r.clones}
}

func (r *rect) Width() int     { return r.width }
func (r *rect) SetWidth(w int) { r.width = w }

// Copyable is an interface type that declares DeepCopy() Copyable.
type Copyable interface {
	DeepCopy() Copyable
	Name() string
}

type named struct{ name string }

func (n *named) DeepCopy() Copyable { return &named{name: n.name + "-deepcopy"} }
func (n *named) Name() string       { return n.name }

// cloneAndDeepCopy has both methods; Clone must win.
type cloneAndDeepCopy struct{ via string }

func (c *cloneAndDeepCopy) Clone() *cloneAndDeepCopy    { return &cloneAndDeepCopy{via: "Clone"} }
func (c *cloneAndDeepCopy) DeepCopy() *cloneAndDeepCopy { return &cloneAndDeepCopy{via: "DeepCopy"} }

// namedInt is a named primitive type with a Clone method, which takes precedence.
type namedInt int

func (n namedInt) Clone() namedInt { return n + 1 }

// wrongReturn has a Clone method that does not return the type itself.
type wrongReturn struct{}

func (*wrongReturn) Clone() any { return &wrongReturn{} }

func TestDefaultClonerInterfaceWithCloneMethod(t *testing.T) {
	t.Parallel()

	cloner := loadingcache.DefaultValueCloner[Shape]()
	clones := 0
	var original Shape = &rect{width: 3, clones: &clones}
	cloned := cloner.CloneValue(original)
	if cloned == original {
		t.Fatal("Expected a different value, got the same pointer")
	}
	if clones != 1 {
		t.Errorf("Clone calls: got %d, want 1", clones)
	}
	original.SetWidth(10)
	if cloned.Width() != 3 {
		t.Errorf("Expected cloned value to remain unchanged, got %d", cloned.Width())
	}
}

func TestDefaultClonerInterfaceWithDeepCopyMethod(t *testing.T) {
	t.Parallel()

	cloner := loadingcache.DefaultValueCloner[Copyable]()
	if got := cloner.CloneValue(&named{name: "x"}).Name(); got != "x-deepcopy" {
		t.Errorf("Expected DeepCopy to be called, got %q", got)
	}
}

func TestDefaultClonerPrefersCloneOverDeepCopy(t *testing.T) {
	t.Parallel()

	cloner := loadingcache.DefaultValueCloner[*cloneAndDeepCopy]()
	if got := cloner.CloneValue(&cloneAndDeepCopy{}).via; got != "Clone" {
		t.Errorf("Expected Clone to be preferred, got %q", got)
	}
}

func TestDefaultClonerNamedPrimitiveWithCloneMethod(t *testing.T) {
	t.Parallel()

	cloner := loadingcache.DefaultValueCloner[namedInt]()
	if got := cloner.CloneValue(1); got != 2 {
		t.Errorf("Expected Clone method to be preferred over NopValueCloner, got %d", got)
	}
}

func TestDefaultClonerInterfaceNil(t *testing.T) {
	t.Parallel()

	t.Run("nil interface", func(t *testing.T) {
		t.Parallel()
		if got := loadingcache.DefaultValueCloner[Shape]().CloneValue(nil); got != nil {
			t.Errorf("Expected nil, got %#v", got)
		}
		if got := loadingcache.DefaultValueCloner[Copyable]().CloneValue(nil); got != nil {
			t.Errorf("Expected nil, got %#v", got)
		}
	})

	t.Run("typed nil", func(t *testing.T) {
		t.Parallel()
		// The method is called with the typed nil and decides how to handle it.
		var typedNil Shape = (*rect)(nil)
		got := loadingcache.DefaultValueCloner[Shape]().CloneValue(typedNil)
		if got == nil {
			t.Fatal("Expected the typed nil returned by Clone, got nil interface")
		}
		if r, ok := got.(*rect); !ok || r != nil {
			t.Errorf("Expected (*rect)(nil), got %#v", got)
		}
	})
}

func TestDefaultClonerUnsupportedTypes(t *testing.T) {
	t.Parallel()

	for name, build := range map[string]func(){
		"any":                   func() { loadingcache.DefaultValueCloner[any]() },
		"error":                 func() { loadingcache.DefaultValueCloner[error]() },
		"interface without it":  func() { loadingcache.DefaultValueCloner[interface{ Width() int }]() },
		"map":                   func() { loadingcache.DefaultValueCloner[map[string]int]() },
		"clone returns another": func() { loadingcache.DefaultValueCloner[*wrongReturn]() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				msg, ok := r.(string)
				if !ok || !strings.HasPrefix(msg, "loadingcache: ") {
					t.Errorf("Expected a descriptive panic from loadingcache, got %#v", r)
				}
			}()
			build()
		})
	}
}

// TestDefaultClonerInterfaceConstructors verifies that every constructor that
// builds the default cloner accepts an interface type declaring Clone.
func TestDefaultClonerInterfaceConstructors(t *testing.T) {
	t.Parallel()

	storage := memstorage.NewInMemoryStorage[uint8, Shape]()
	src := &source.FunctionsSource[uint8, Shape]{
		GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, Shape], error) {
			entries := make([]*loadingcache.CacheEntry[uint8, Shape], len(keys))
			for i, key := range keys {
				entries[i] = &loadingcache.CacheEntry[uint8, Shape]{
					Entry:     loadingcache.Entry[uint8, Shape]{Key: key, Value: &rect{width: int(key)}},
					ExpiresAt: time.Now().Add(time.Hour),
				}
			}
			return entries, nil
		},
	}
	cache := loadingcache.LoadingCache[uint8, Shape]{
		Storage: storage,
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
	}
	indexed := loadingcache.NewIndexedLoadingCache(cache, omcindex.NewOnMemoryIndex[uint8, uint8](nil))
	if indexed == nil {
		t.Fatal("NewIndexedLoadingCache returned nil")
	}

	ctx := t.Context()
	got, err := cache.GetOrLoad(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	got.Value.SetWidth(100)

	got, err = cache.GetOrLoad(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if w := got.Value.Width(); w != 5 {
		t.Errorf("cached value changed by mutating a returned value: got width %d, want 5", w)
	}
}
