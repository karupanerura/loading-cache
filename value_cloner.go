package loadingcache

import "reflect"

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
// It returns a NopValueCloner if the value type does not have Clone or DeepCopy method.
// The value type must implement Clone or DeepCopy method.
func DefaultValueCloner[V ValueConstraint]() ValueCloner[V] {
	var zero V
	return defaultValueClonerAny[V](zero)
}

func defaultValueClonerAny[V ValueConstraint](v any) ValueCloner[V] {
	type cloner interface {
		Clone() V
	}
	type deepCopier interface {
		DeepCopy() V
	}

	switch v.(type) {
	case cloner:
		return ValueClonerFunc[V](func(v V) V {
			var a any = v
			return a.(cloner).Clone()
		})

	case deepCopier:
		return ValueClonerFunc[V](func(v V) V {
			var a any = v
			return a.(deepCopier).DeepCopy()
		})

	default:
		return defaultValueClonerReflect[V](reflect.ValueOf(v).Type())
	}
}

func defaultValueClonerReflect[V ValueConstraint](typ reflect.Type) ValueCloner[V] {
	switch typ.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer:
		return NopValueCloner[V]{}
	default:
		panic("value type does not have Clone or DeepCopy method")
	}
}
