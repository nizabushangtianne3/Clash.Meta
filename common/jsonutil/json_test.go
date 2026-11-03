package jsonutil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendStringMatchesEncodingJSON(t *testing.T) {
	values := []string{
		"",
		`quote"slash\\html<&>line\n`,
		"plain ascii 0123456789",
		"DEL \u007f remains plain",
		"unicode\u2028\u2029\ufffd",
		"invalid\xffutf8",
	}

	for _, value := range values {
		expected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(AppendString(nil, value)))
		require.Equal(t, len(expected), StringLen(value), "size of %q", value)
	}
}

func TestStringLenSizesEveryEscape(t *testing.T) {
	probes := []string{
		"\x00\x01\x1f",
		"tab\there",
		"\u2028\u2029\u007f\u0080",
		"plain ascii",
		"\xff\xfe",
	}
	for _, probe := range probes {
		require.Equal(t, len(AppendString(nil, probe)), StringLen(probe), "size of %q", probe)
	}
}
