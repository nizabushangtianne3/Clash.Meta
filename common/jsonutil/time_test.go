package jsonutil

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAppendTimeMatchesEncodingJSON(t *testing.T) {
	values := []time.Time{
		{},
		time.Date(2026, 11, 4, 5, 6, 7, 0, time.UTC),
		time.Date(2026, 11, 4, 5, 6, 7, 123456789, time.FixedZone("test", 8*3600)),
	}
	for _, value := range values {
		expected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(AppendTime(nil, value)))
	}
}
