package provider

import (
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The alternation a health check filters by is compiled when it is registered,
// so a check round no longer rebuilds what registration already built.
func TestHealthCheckFilterCompiledAtRegistration(t *testing.T) {
	option := &extraOption{filters: map[string]struct{}{}}
	splitAndAddFiltersToExtra("(?i)hk`(?i)singapore", option)
	require.NotNil(t, option.filter)

	for _, name := range []string{"HK-01", "hk", "Singapore-02", "singapore"} {
		match, err := option.filter.MatchString(name)
		require.NoError(t, err)
		assert.True(t, match, name)
	}
	for _, name := range []string{"Tokyo-01", "los angeles", ""} {
		match, err := option.filter.MatchString(name)
		require.NoError(t, err)
		assert.False(t, match, name)
	}

	// the same filter registered again leaves the compiled set alone
	before := option.filter
	splitAndAddFiltersToExtra("(?i)hk", option)
	assert.Same(t, before, option.filter)

	// a filter that is new extends it
	splitAndAddFiltersToExtra("(?i)tokyo", option)
	assert.Len(t, option.filters, 3)
	match, err := option.filter.MatchString("Tokyo-01")
	require.NoError(t, err)
	assert.True(t, match)
}

func TestHealthCheckWithoutFilter(t *testing.T) {
	option := &extraOption{filters: map[string]struct{}{}}
	splitAndAddFiltersToExtra("  ` `", option)
	assert.Nil(t, option.filter)

	splitAndAddFiltersToExtra("   ", option)
	assert.Nil(t, option.filter)
}

// The compiled set has to answer what joining the filters with an alternation
// answers, which is what a check round used to build on every pass.
func TestHealthCheckFilterMatchesJoinedAlternation(t *testing.T) {
	option := &extraOption{filters: map[string]struct{}{}}
	splitAndAddFiltersToExtra("(?i)hk`(?i)singapore", option)
	reference := regexp2.MustCompile("(?i)hk|(?i)singapore", regexp2.None)

	names := []string{"HK-01", "hk", "hkx", "Singapore-02", "SINGAPORE", "Tokyo-01", "", "香港-HK"}
	for _, name := range names {
		got, err := option.filter.MatchString(name)
		require.NoError(t, err)
		want, err := reference.MatchString(name)
		require.NoError(t, err)
		assert.Equal(t, want, got, name)
	}
}
