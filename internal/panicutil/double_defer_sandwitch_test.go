package panicutil_test

import (
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/karupanerura/loading-cache/internal/panicutil"
	"github.com/sourcegraph/conc/panics"
)

func TestDDS(t *testing.T) {
	t.Parallel()

	t.Run("Normal return with no error", func(t *testing.T) {
		t.Parallel()

		err := panicutil.DDS(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("Normal return with error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("expected error")
		err := panicutil.DDS(func() error {
			return expectedErr
		})
		if err != expectedErr {
			t.Errorf("expected error %v, got: %v", expectedErr, err)
		}
	})

	t.Run("Panic with string", func(t *testing.T) {
		t.Parallel()

		err := panicutil.DDS(func() error {
			panic("test panic")
		})
		var recoveredErr *panics.ErrRecovered
		if !errors.As(err, &recoveredErr) {
			t.Fatalf("expected error to be of type *panics.ErrRecovered, got: %T", err)
		}
		if recoveredErr.Value != "test panic" {
			t.Errorf("expected panic value 'test panic', got: %v", err)
		}
	})

	t.Run("Panic with error", func(t *testing.T) {
		t.Parallel()

		customErr := errors.New("custom error")
		err := panicutil.DDS(func() error {
			panic(customErr)
		})
		var recoveredErr *panics.ErrRecovered
		if !errors.As(err, &recoveredErr) {
			t.Fatalf("expected error to be of type *panics.ErrRecovered, got: %T", err)
		}
		if recoveredErr.Value != customErr {
			t.Errorf("expected panic value custom error, got: %v", err)
		}
	})

	t.Run("Runtime.Goexit", func(t *testing.T) {
		t.Parallel()

		var wg sync.WaitGroup
		var err error

		wg.Add(1)
		go func() {
			defer wg.Done()
			err = panicutil.DDS(func() error {
				runtime.Goexit()
				t.Log("This should not be printed")
				return nil // unreachable
			})
		}()
		wg.Wait()

		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("Nested DDS", func(t *testing.T) {
		t.Parallel()

		customErr := errors.New("inner error")
		err := panicutil.DDS(func() error {
			return panicutil.DDS(func() error {
				return customErr
			})
		})
		if err != customErr {
			t.Errorf("expected error %v, got: %v", customErr, err)
		}
	})

	t.Run("Nested DDS with panic", func(t *testing.T) {
		t.Parallel()

		err := panicutil.DDS(func() error {
			return panicutil.DDS(func() error {
				panic("inner panic")
			})
		})
		var recoveredErr *panics.ErrRecovered
		if !errors.As(err, &recoveredErr) {
			t.Fatalf("expected error to be of type *panics.ErrRecovered, got: %T", err)
		}
		if recoveredErr.Value != "inner panic" {
			t.Errorf("expected panic value 'inner panic', got: %v", err)
		}
	})

	t.Run("Runtime.Goexit with nested DDS", func(t *testing.T) {
		t.Parallel()

		var wg sync.WaitGroup
		var err error

		wg.Add(1)
		go func() {
			defer wg.Done()
			err = panicutil.DDS(func() error {
				return panicutil.DDS(func() error {
					runtime.Goexit()
					return nil // unreachable
				})
			})
		}()
		wg.Wait()

		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}
