package jsonutil

import (
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/require"
)

func TestAppendUUIDMatchesCanonicalForm(t *testing.T) {
	value := uuid.Must(uuid.FromString("0f8FAD5b-D9CB-469f-a165-70867728950E"))
	require.Equal(t, `"0f8fad5b-d9cb-469f-a165-70867728950e"`, string(AppendUUID(nil, value)))
}
