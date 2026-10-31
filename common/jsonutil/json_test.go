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
		"unicode\u2028\u2029\ufffd",
		"invalid\xffutf8",
	}

	for _, value := range values {
		expected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(AppendString(nil, value)))
	}
}
