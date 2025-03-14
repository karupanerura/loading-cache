package panicutil

import (
	"github.com/sourcegraph/conc/panics"
)

// DDS runs the function with double defer sandwich. It recovers from panics and returns them as errors.
// If the function returns normally, it returns the error value returned from the given function.
// If the function panics, it returns the recovered panic value as an error as *panics.ErrRecovered.
// If the function calls runtime.Goexit, it returns nil.
func DDS(f func() error) error {
	var dds DoubleDeferSandwich
	return dds.Invoke(f)
}

// DoubleDeferSandwich is a struct that provides a double defer sandwich mechanism.
type DoubleDeferSandwich struct {
	// OnGoexit is a function that is called when the function calls runtime.Goexit.
	// If the function does not call runtime.Goexit, this function is not called.
	OnGoexit func()
}

// Invoke runs the function with double defer sandwich. It recovers from panics and returns them as errors.
// If the function returns normally, it returns the error value returned from the given function.
// If the function panics, it returns the recovered panic value as an error as *panics.ErrRecovered.
// If the function calls runtime.Goexit, it calls the OnGoexit function.
func (dds *DoubleDeferSandwich) Invoke(f func() error) (err error) {
	var (
		normalReturn bool
		recovered    bool
		panicValue   panics.Recovered
	)
	defer func() {
		switch {
		case normalReturn:
			return
		case recovered:
			err = panicValue.AsError()
		default:
			if dds.OnGoexit != nil {
				dds.OnGoexit()
			}
		}
	}()
	func() {
		defer func() {
			panicValue = panics.NewRecovered(2, recover())
		}()
		err = f()
		normalReturn = true
	}()
	if !normalReturn {
		recovered = true
	}
	return
}
