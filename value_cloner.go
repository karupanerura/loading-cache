package loadingcache

import (
	"fmt"
	"reflect"
)

// ValueCloner is an interface for cloning values.
// It is used to clone values when they are stored in the cache.
// The CloneValue method should return a deep copy of the input value.
type ValueCloner[V ValueConstraint] interface {
	CloneValue(V) V
}

// ValueClonerFunc is a function type that implements the ValueCloner interface.
type ValueClonerFunc[V ValueConstraint] func(v V) V

// CloneValue calls the function.
func (f ValueClonerFunc[V]) CloneValue(v V) V {
	return f(v)
}

// NopValueCloner is a value cloner that does not clone values.
// It is used when values do not need to be cloned. (e.g. when the values are primitive types or immutable usage)
type NopValueCloner[V ValueConstraint] struct{}

// CloneValue returns the input value.
func (NopValueCloner[V]) CloneValue(v V) V {
	return v
}

// DefaultValueCloner returns a default cloner for the given value type.
// The cloner is chosen from the static type V, in this order:
//
//  1. If V has a method Clone() V, the cloner calls it.
//  2. If V has a method DeepCopy() V, the cloner calls it.
//  3. For bool, numeric, string, and unsafe.Pointer types, it returns a NopValueCloner.
//
// The methods must return V itself; methods returning any other type are ignored.
// Methods take precedence over rule 3, so a named primitive type with a Clone
// method is cloned by that method.
//
// V may be an interface type that declares Clone() V or DeepCopy() V.
// A nil interface value is returned as is; any other value, including a typed
// nil pointer stored in the interface, is passed to the method, which decides
// how to handle it. Interface types that do not declare these methods, such
// as any and error, are not supported even if the values stored in them have
// such methods, because the cloner is chosen from V and not from each value.
//
// It panics for any unsupported type. Such types need an explicit ValueCloner.
func DefaultValueCloner[V ValueConstraint]() ValueCloner[V] {
	type cloner interface {
		Clone() V
	}
	type deepCopier interface {
		DeepCopy() V
	}

	typ := reflect.TypeFor[V]()
	switch {
	case typ.Implements(reflect.TypeFor[cloner]()):
		return ValueClonerFunc[V](func(v V) V {
			c, ok := any(v).(cloner)
			if !ok {
				// Only a nil interface value fails the assertion.
				return v
			}
			return c.Clone()
		})

	case typ.Implements(reflect.TypeFor[deepCopier]()):
		return ValueClonerFunc[V](func(v V) V {
			c, ok := any(v).(deepCopier)
			if !ok {
				// Only a nil interface value fails the assertion.
				return v
			}
			return c.DeepCopy()
		})
	}

	switch typ.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer:
		return NopValueCloner[V]{}
	case reflect.Interface:
		panic(fmt.Sprintf("loadingcache: interface type %v does not declare Clone() %v or DeepCopy() %v; set a ValueCloner explicitly", typ, typ, typ))
	default:
		panic(fmt.Sprintf("loadingcache: value type %v has neither Clone() %v nor DeepCopy() %v method; set a ValueCloner explicitly", typ, typ, typ))
	}
}
