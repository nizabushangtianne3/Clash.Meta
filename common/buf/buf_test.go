package buf

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fork's Buffer is sing's, so the constructors only carry the capacity and
// length contract the callers of every read and write depend on.
func TestNewAndNewSizeHandUsAnEmptyBuffer(t *testing.T) {
	fresh := New()
	require.NotNil(t, fresh)
	assert.Zero(t, fresh.Len())
	assert.Equal(t, BufferSize, fresh.Cap())

	sized := NewSize(64)
	require.NotNil(t, sized)
	assert.Zero(t, sized.Len())
	assert.Equal(t, 64, sized.Cap())

	packet := NewPacket()
	require.NotNil(t, packet)
	assert.Zero(t, packet.Len())
	assert.Positive(t, packet.Cap())
}

// As wraps a slice that already holds data; With hands back an empty buffer
// over the same memory. The difference is what a caller writes first.
func TestAsCarriesTheDataAndWithDoesNot(t *testing.T) {
	payload := []byte("payload")

	as := As(payload)
	assert.Equal(t, len(payload), as.Len())
	assert.Equal(t, payload, as.Bytes())

	with := With(payload)
	assert.Zero(t, with.Len())
	assert.Equal(t, len(payload), with.Cap())
	assert.Empty(t, with.Bytes())

	n, err := with.Write([]byte("abcd"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	assert.Equal(t, "abcd", string(with.Bytes()))
}

// With aliases the slice it was handed, so the caller must not keep using it:
// whatever the wrapper writes lands in the caller's memory.
func TestWithAliasesTheCallersSlice(t *testing.T) {
	source := []byte("source")
	with := With(source)

	_, err := with.WriteString("X")
	require.NoError(t, err)

	assert.Equal(t, "Xource", string(source))
	assert.Equal(t, "X", string(with.Bytes()))
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	b := New()

	n, err := b.Write([]byte("hello "))
	require.NoError(t, err)
	require.Equal(t, len("hello "), n)

	n, err = b.WriteString("world")
	require.NoError(t, err)
	require.Equal(t, len("world"), n)
	assert.Equal(t, "hello world", string(b.Bytes()))
	assert.Equal(t, b.Len(), len("hello world"))

	got := make([]byte, len("hello "))
	m, err := b.Read(got)
	require.NoError(t, err)
	require.Equal(t, len("hello "), m)
	assert.Equal(t, "hello ", string(got))
	assert.Equal(t, "world", string(b.Bytes()))

	b.Release()
}

// Truncate and Resize are how a caller hands back part of the space it
// reserved for a header it has not written yet.
func TestTruncateAndResizeMoveTheWindow(t *testing.T) {
	b := New()
	_, err := b.WriteString("0123456789")
	require.NoError(t, err)

	b.Truncate(4)
	assert.Equal(t, 4, b.Len())
	assert.Equal(t, "0123", string(b.Bytes()))

	// Resize opens a window: start is absolute and the second argument is a
	// length, so this is the data between 2 and 5.
	b.Resize(2, 3)
	assert.Equal(t, 3, b.Len())
	assert.Equal(t, "234", string(b.Bytes()))

	b.Release()
}

// Reset keeps the allocation for the next datagram; FullReset hands the space
// back to the pool.
func TestResetKeepsTheAllocation(t *testing.T) {
	b := New()
	_, err := b.WriteString("some payload")
	require.NoError(t, err)
	capacity := b.Cap()

	b.Reset()
	assert.Zero(t, b.Len())
	assert.Equal(t, capacity, b.Cap())
	assert.True(t, b.IsEmpty())

	_, err = b.WriteString("reuse")
	require.NoError(t, err)
	assert.Equal(t, "reuse", string(b.Bytes()))

	b.FullReset()
	assert.Zero(t, b.Len())

	b.Release()
}

func TestReleaseMultiReleasesEveryBuffer(t *testing.T) {
	first := New()
	second := NewSize(64)
	require.NotNil(t, first)
	require.NotNil(t, second)

	ReleaseMulti([]*Buffer{first, second})

	// A released buffer must not surprise a caller that still holds it.
	assert.Zero(t, first.Len())
	assert.Zero(t, second.Len())
}

func TestErrorReturnsWhatItWasHanded(t *testing.T) {
	sentinel := errors.New("boom")

	assert.Equal(t, sentinel, Error(struct{}{}, sentinel))
	assert.NoError(t, Error(struct{}{}, nil))
}

func TestMustPanicsOnTheFirstError(t *testing.T) {
	sentinel := errors.New("boom")

	Must()
	Must(nil, nil)

	assert.PanicsWithError(t, sentinel.Error(), func() { Must(nil, sentinel) })
	assert.PanicsWithError(t, sentinel.Error(), func() { Must(sentinel, errors.New("second")) })
}

func TestMust1AndMust2(t *testing.T) {
	assert.Equal(t, 7, Must1(7, nil))
	assert.Panics(t, func() { Must1(7, errors.New("boom")) })

	first, second := Must2("a", 1, nil)
	assert.Equal(t, "a", first)
	assert.Equal(t, 1, second)

	assert.Panics(t, func() { Must2("a", 1, errors.New("boom")) })
}

// The alias is the whole point of the package: a caller that passes a Buffer
