package pagedbtree

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/fs"
	"github.com/teapotovh/teapot/lib/run"
)

// ErrInvalidSize reports data whose size does not match the value.
var ErrInvalidSize = errors.New("invalid value size")

// Keyed is a value that knows its key.
type Keyed interface {
	// Key returns the key for the value.
	Key() string
}

// Codec encodes a value to bytes and decodes bytes to a value.
type Codec[T any] interface {
	// Encode returns the bytes for value.
	Encode(value T) ([]byte, error)
	// Decode returns the value in data.
	Decode(data []byte) (T, error)
}

// Raw stores the memory bytes of a value as they are.
// It does not follow pointers or encode referenced data.
type Raw[T any] struct{}

// Encode implements Codec.
func (Raw[T]) Encode(value T) ([]byte, error) {
	data := make([]byte, unsafe.Sizeof(value))
	copy(data, rawBytes(&value))

	return data, nil
}

// Decode implements Codec.
func (Raw[T]) Decode(data []byte) (T, error) {
	var value T

	size := unsafe.Sizeof(value)
	if uintptr(len(data)) != size {
		return value, ErrInvalidSize
	}

	if size == 0 {
		return value, nil
	}

	copy(rawBytes(&value), data)

	return value, nil
}

//nolint:gosec // Raw copies memory on purpose.
func rawBytes[T any](value *T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(value)), unsafe.Sizeof(*value))
}

// Tree is the typed B+tree store.
type Tree[T Keyed] struct {
	store *Bytes
	codec Codec[T]
}

// OpenTree opens or creates a typed store.
func OpenTree[T Keyed](fs fs.FS, cfg Config, codec Codec[T]) (*Tree[T], error) {
	store, err := Open(fs, cfg)
	if err != nil {
		return nil, err
	}

	return &Tree[T]{store: store, codec: codec}, nil
}

// Load returns the value for key.
func (t *Tree[T]) Load(key string) (T, bool, error) {
	var zero T

	data, ok, err := t.store.Load(key)
	if err != nil || !ok {
		return zero, ok, err
	}

	value, err := t.codec.Decode(data)
	if err != nil {
		return zero, false, fmt.Errorf("could not decode value: %w", err)
	}

	return value, true, nil
}

// Store writes a value, using the key from value.Key().
func (t *Tree[T]) Store(value T) error {
	data, err := t.codec.Encode(value)
	if err != nil {
		return fmt.Errorf("could not encode value: %w", err)
	}

	return t.store.Store(value.Key(), data)
}

// Delete removes key from the store.
func (t *Tree[T]) Delete(key string) error {
	return t.store.Delete(key)
}

// All iterates over all entries in key order.
func (t *Tree[T]) All() iter.Seq2[string, T] {
	return t.scan(t.store.All())
}

// From iterates over entries with a key greater than or equal to from.
func (t *Tree[T]) From(from string) iter.Seq2[string, T] {
	return t.scan(t.store.From(from))
}

// Between iterates over entries with a key in [from, to).
func (t *Tree[T]) Between(from, to string) iter.Seq2[string, T] {
	return t.scan(t.store.Between(from, to))
}

// Run implements run.Runnable.
func (t *Tree[T]) Run(step run.Step) error {
	return t.store.Run(step)
}

// Metrics implements observability.Metrics.
func (t *Tree[T]) Metrics() []prometheus.Collector {
	return t.store.Metrics()
}

func (t *Tree[T]) scan(seq iter.Seq2[string, []byte]) iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		for key, data := range seq {
			value, err := t.codec.Decode(data)
			if err != nil {
				return
			}

			if !yield(key, value) {
				return
			}
		}
	}
}
