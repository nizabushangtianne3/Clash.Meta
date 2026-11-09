package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The header the host shows the traffic bar from is a subscription's own
// free-form string, so the parse has to survive whatever a机场 writes in it.
func TestNewSubscriptionInfoReadsWhatTheHeaderCarries(t *testing.T) {
	got := NewSubscriptionInfo("upload=123; download=456; total=789; expire=1730000000")
	require.NotNil(t, got)
	require.EqualValues(t, 123, got.Upload)
	require.EqualValues(t, 456, got.Download)
	require.EqualValues(t, 789, got.Total)
	require.EqualValues(t, 1730000000, got.Expire)
}

func TestNewSubscriptionInfoSurvivesTheShapesProvidersSend(t *testing.T) {
	// Spacing and case are the provider's choice, and a byte count is often
	// written as a float.
	got := NewSubscriptionInfo(" Upload = 1.9 ;Download=2; Total = 3.0 ; Expire = 4e3 ; Unknown = 9 ; Broken")
	require.NotNil(t, got)
	require.EqualValues(t, 1, got.Upload, "a float byte count truncates")
	require.EqualValues(t, 2, got.Download)
	require.EqualValues(t, 3, got.Total)
	require.EqualValues(t, 4000, got.Expire, "an exponent form is a number too")
}

// A field that does not parse costs that field, not the whole header.
func TestNewSubscriptionInfoSkipsOnlyWhatItCannotRead(t *testing.T) {
	got := NewSubscriptionInfo("upload=abc; download=5")
	require.NotNil(t, got)
	require.Zero(t, got.Upload, "a field that does not parse stays zero rather than failing the read")
	require.EqualValues(t, 5, got.Download, "the fields after it still land")

	require.NotNil(t, NewSubscriptionInfo(""), "an empty header answers an empty info, not nil")
}
