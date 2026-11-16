package imap

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViewBasics(t *testing.T) {
	t.Parallel()

	view := NewView(1234, []uint32{1, 2, 3, 4, 5})

	require.Equal(t, uint32(1234), view.UIDValidity())
	require.Equal(t, uint32(5), view.Count())
	require.Equal(t, uint32(5), view.HighestUID())
	require.Equal(t, []uint32{1, 2, 3, 4, 5}, view.UIDs())

	for i, uid := range []uint32{1, 2, 3, 4, 5} {
		got, ok := view.UIDAt(uint32(i + 1))
		require.True(t, ok)
		require.Equal(t, uid, got)

		seq, ok := view.Seq(uid)
		require.True(t, ok)
		require.Equal(t, uint32(i+1), seq)
	}

	_, ok := view.UIDAt(0)
	require.False(t, ok)

	_, ok = view.UIDAt(6)
	require.False(t, ok)

	_, ok = view.Seq(6)
	require.False(t, ok)
}

func TestViewAppendAndRemove(t *testing.T) {
	t.Parallel()

	view := NewView(7, []uint32{1, 2, 3, 4, 5})

	require.NoError(t, view.Append(6))
	require.Equal(t, uint32(6), view.Count())
	require.Equal(t, uint32(6), view.HighestUID())

	seq, ok := view.Remove(3)
	require.True(t, ok)
	require.Equal(t, uint32(3), seq)
	require.Equal(t, uint32(5), view.Count())
	require.Equal(t, []uint32{1, 2, 4, 5, 6}, view.UIDs())

	got, ok := view.UIDAt(3)
	require.True(t, ok)
	require.Equal(t, uint32(4), got)

	seq, ok = view.Seq(4)
	require.True(t, ok)
	require.Equal(t, uint32(3), seq)

	seq, ok = view.Remove(1)
	require.True(t, ok)
	require.Equal(t, uint32(1), seq)
	require.Equal(t, []uint32{2, 4, 5, 6}, view.UIDs())

	seq, ok = view.Remove(6)
	require.True(t, ok)
	require.Equal(t, uint32(4), seq)
	require.Equal(t, []uint32{2, 4, 5}, view.UIDs())
	require.Equal(t, uint32(6), view.HighestUID())

	_, ok = view.Remove(3)
	require.False(t, ok)

	seq, ok = view.Seq(5)
	require.True(t, ok)
	require.Equal(t, uint32(3), seq)
}

func TestViewInvalidAppend(t *testing.T) {
	t.Parallel()

	view := NewView(1, []uint32{1, 2, 3})

	require.ErrorIs(t, view.Append(3), ErrInvalidUID)
	require.ErrorIs(t, view.Append(2), ErrInvalidUID)
	require.ErrorIs(t, view.Append(0), ErrInvalidUID)
	require.Equal(t, []uint32{1, 2, 3}, view.UIDs())

	require.NoError(t, view.Append(4))
	require.Equal(t, []uint32{1, 2, 3, 4}, view.UIDs())
}

func TestViewReset(t *testing.T) {
	t.Parallel()

	view := NewView(1, []uint32{5, 3, 3, 0, 4})

	require.Equal(t, uint32(3), view.Count())
	require.Equal(t, []uint32{3, 4, 5}, view.UIDs())
	require.Equal(t, uint32(5), view.HighestUID())

	view.Reset(2, []uint32{10, 11})

	require.Equal(t, uint32(2), view.UIDValidity())
	require.Equal(t, uint32(2), view.Count())
	require.Equal(t, uint32(11), view.HighestUID())
	require.Equal(t, []uint32{10, 11}, view.UIDs())

	seq, ok := view.Seq(11)
	require.True(t, ok)
	require.Equal(t, uint32(2), seq)

	require.NoError(t, view.Append(12))
	require.Equal(t, []uint32{10, 11, 12}, view.UIDs())
}

func TestViewEmpty(t *testing.T) {
	t.Parallel()

	view := NewView(1, nil)

	require.Equal(t, uint32(0), view.Count())
	require.Equal(t, uint32(0), view.HighestUID())
	require.Empty(t, view.UIDs())

	_, ok := view.UIDAt(1)
	require.False(t, ok)

	_, ok = view.Remove(1)
	require.False(t, ok)

	require.NoError(t, view.Append(1))
	require.Equal(t, []uint32{1}, view.UIDs())
}

//nolint:gosec
func TestViewRandomAgainstSlice(t *testing.T) {
	t.Parallel()

	const iterations = 2000

	rng := rand.New(rand.NewSource(42))
	view := NewView(99, nil)

	var model []uint32

	nextUID := uint32(1)

	for range iterations {
		if len(model) == 0 || rng.Intn(100) < 60 {
			require.NoError(t, view.Append(nextUID))
			model = append(model, nextUID)
			nextUID++

			continue
		}

		index := rng.Intn(len(model))
		uid := model[index]

		seq, ok := view.Remove(uid)
		require.True(t, ok)
		require.Equal(t, uint32(index+1), seq)

		model = append(model[:index], model[index+1:]...)
	}

	require.Equal(t, uint32(len(model)), view.Count())
	require.Equal(t, nextUID-1, view.HighestUID())
	require.Equal(t, model, view.UIDs())

	for i, uid := range model {
		got, ok := view.UIDAt(uint32(i + 1))
		require.True(t, ok)
		require.Equal(t, uid, got)

		seq, ok := view.Seq(uid)
		require.True(t, ok)
		require.Equal(t, uint32(i+1), seq)
	}

	_, ok := view.UIDAt(uint32(len(model) + 1))
	require.False(t, ok)
}
