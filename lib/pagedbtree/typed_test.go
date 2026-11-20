package pagedbtree

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

type jsonCodec[T any] struct{}

func (jsonCodec[T]) Encode(value T) ([]byte, error) {
	return json.Marshal(value)
}

func (jsonCodec[T]) Decode(data []byte) (T, error) {
	var value T

	err := json.Unmarshal(data, &value)

	return value, err
}

func TestRawCodec(t *testing.T) {
	t.Parallel()

	type value struct {
		Number uint64
		Flags  [4]byte
	}

	codec := Raw[value]{}
	want := value{Number: 42, Flags: [4]byte{1, 2, 3, 4}}

	data, err := codec.Encode(want)
	if err != nil {
		t.Fatal(err)
	}

	got, err := codec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Fatalf("Decode(Encode(value)) = %#v, want %#v", got, want)
	}

	if _, err := codec.Decode(data[1:]); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("Decode(short data) error = %v, want %v", err, ErrInvalidSize)
	}
}

func TestTypedTree(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)

	tree, err := OpenTree(cfg, jsonCodec[typedValue]{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = tree.Close() })

	values := []typedValue{
		{key: "a", data: "one"},
		{key: "b", data: "two"},
	}

	for _, value := range values {
		if err := tree.Store(value); err != nil {
			t.Fatal(err)
		}
	}

	got, ok, err := tree.Load("b")
	if err != nil {
		t.Fatal(err)
	}

	if !ok || got != values[1] {
		t.Fatalf("Load(b) = %#v, want %#v", got, values[1])
	}

	keys := []string{}
	for key := range tree.All() {
		keys = append(keys, key)
	}

	if fmt.Sprint(keys) != "[a b]" {
		t.Fatalf("All() keys = %v", keys)
	}

	if err := tree.Delete("a"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := tree.Load("a"); err != nil || ok {
		t.Fatalf("Load(a) after Delete = ok %v, err %v", ok, err)
	}
}

type typedValue struct {
	key  string
	data string
}

func (v typedValue) Key() string {
	return v.key
}

func (v typedValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key  string `json:"key"`
		Data string `json:"data"`
	}{Key: v.key, Data: v.data})
}

func (v *typedValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Key  string `json:"key"`
		Data string `json:"data"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.key = raw.Key
	v.data = raw.Data

	return nil
}
