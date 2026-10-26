package constant

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The enums reach the host as their name, so the text and the json encoder
// have to agree on it for every value, including the unknown fallbacks.

type textEnum interface {
	MarshalText() ([]byte, error)
	String() string
}

func TestEnumTextIsItsName(t *testing.T) {
	enums := []textEnum{
		DNSNormal, DNSFakeIP, DNSMapping, DNSHosts, DNSMode(99),
		DualStack, IPv4Only, IPv6Only, IPv4Prefer, IPv6Prefer,
		FilterBlackList, FilterWhiteList, FilterRule, FilterMode(99),
		TunGvisor, TunSystem, TunMixed, TunMips, TUNStack(99),
	}

	for _, enum := range enums {
		marshaled, err := enum.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, enum.String(), string(marshaled))

		encoded, err := json.Marshal(enum)
		require.NoError(t, err)
		assert.Equal(t, `"`+enum.String()+`"`, string(encoded))
	}
}
