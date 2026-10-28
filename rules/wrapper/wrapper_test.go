package wrapper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	C "github.com/metacubex/mihomo/constant"
)

// The helper is only handed to the wrapped rule, so a nil interface value is
// enough here.
var noHelper C.RuleMatchHelper

type stubRule struct {
	matched bool
	adapter string
	calls   int
}

func (r *stubRule) RuleType() C.RuleType { return C.Domain }
func (r *stubRule) Match(*C.Metadata, C.RuleMatchHelper) (bool, string) {
	r.calls++
	return r.matched, r.adapter
}
func (r *stubRule) Adapter() string         { return r.adapter }
func (r *stubRule) Payload() string         { return "payload" }
func (r *stubRule) ProviderNames() []string { return nil }

func newWrapper(t *testing.T, rule C.Rule) *RuleWrapper {
	t.Helper()
	return NewRuleWrapper(rule).(*RuleWrapper)
}

func TestRuleWrapperCountsHitsAndMisses(t *testing.T) {
	wrapper := newWrapper(t, &stubRule{matched: true, adapter: "hit"})
	for range 20 {
		ok, adapter := wrapper.Match(nil, noHelper)
		require.True(t, ok)
		require.Equal(t, "hit", adapter)
	}

	missWrapper := newWrapper(t, &stubRule{})
	for range 5 {
		ok, adapter := missWrapper.Match(nil, noHelper)
		require.False(t, ok)
		require.Empty(t, adapter)
	}

	assert.EqualValues(t, 20, wrapper.HitCount())
	assert.EqualValues(t, 0, wrapper.MissCount())
	assert.EqualValues(t, 0, missWrapper.HitCount())
	assert.EqualValues(t, 5, missWrapper.MissCount())
}

// A disabled rule must not reach the wrapped rule, and must not count either.
func TestRuleWrapperShortCircuitsWhenDisabled(t *testing.T) {
	rule := &stubRule{matched: true, adapter: "hit"}
	wrapper := newWrapper(t, rule)
	wrapper.SetDisabled(true)

	ok, adapter := wrapper.Match(nil, noHelper)
	assert.False(t, ok)
	assert.Empty(t, adapter)
	assert.Equal(t, 0, rule.calls)
	assert.EqualValues(t, 0, wrapper.HitCount())
	assert.EqualValues(t, 0, wrapper.MissCount())

	wrapper.SetDisabled(false)
	ok, adapter = wrapper.Match(nil, noHelper)
	require.True(t, ok)
	require.Equal(t, "hit", adapter)
	assert.Equal(t, 1, rule.calls)
}

// The clock is read on every sixteenth event, so a hot rule pays for it once
// per sixteen matches while the timestamp still moves over time.
func TestRuleWrapperFoldsTheClockRead(t *testing.T) {
	wrapper := newWrapper(t, &stubRule{matched: true, adapter: "hit"})

	wrapper.Hit()
	first := wrapper.HitAt()
	require.False(t, first.IsZero())

	// Events two through sixteen must not refresh the timestamp.
	for range 15 {
		wrapper.Hit()
	}
	assert.True(t, first.Equal(wrapper.HitAt()))

	// The seventeenth folds back onto the clock.
	wrapper.Hit()
	assert.EqualValues(t, 17, wrapper.HitCount())
	assert.False(t, wrapper.HitAt().Before(first))
}

func TestRuleWrapperMissFoldsTheClockToo(t *testing.T) {
	wrapper := newWrapper(t, &stubRule{})

	wrapper.Miss()
	first := wrapper.MissAt()
	require.False(t, first.IsZero())
	for range 15 {
		wrapper.Miss()
	}
	assert.True(t, first.Equal(wrapper.MissAt()))

	wrapper.Miss()
	assert.EqualValues(t, 17, wrapper.MissCount())
	assert.False(t, wrapper.MissAt().Before(first))
}

func TestRuleWrapperUnwrapsToTheWrappedRule(t *testing.T) {
	rule := &stubRule{}
	wrapper := newWrapper(t, rule)

	assert.Same(t, rule, wrapper.Unwrap())
	assert.Equal(t, C.Domain, wrapper.RuleType())
	assert.Equal(t, "payload", wrapper.Payload())
	assert.Nil(t, wrapper.ProviderNames())
}

func TestAtomicTimeRoundTrip(t *testing.T) {
	var at atomicTime

	at.Store(time.Unix(1700000000, 123))
	assert.Equal(t, time.Unix(1700000000, 123), at.Load())

	text, err := at.MarshalText()
	require.NoError(t, err)
	var decoded atomicTime
	require.NoError(t, decoded.UnmarshalText(text))
	assert.Equal(t, at.Load(), decoded.Load())

	previous := at.Swap(time.Unix(1800000000, 0))
	assert.Equal(t, time.Unix(1700000000, 123), previous)
	assert.Equal(t, time.Unix(1800000000, 0), at.Load())

	assert.True(t, at.CompareAndSwap(time.Unix(1800000000, 0), time.Unix(1900000000, 0)))
	assert.Equal(t, time.Unix(1900000000, 0), at.Load())
	assert.False(t, at.CompareAndSwap(time.Unix(1800000000, 0), time.Unix(0, 0)))
	assert.Equal(t, time.Unix(1900000000, 0), at.Load())
}

func TestAtomicTimeRejectsAMalformedTimestamp(t *testing.T) {
	var at atomicTime
	require.Error(t, at.UnmarshalText([]byte("not a timestamp")))
	assert.Equal(t, time.Unix(0, 0), at.Load())
}
