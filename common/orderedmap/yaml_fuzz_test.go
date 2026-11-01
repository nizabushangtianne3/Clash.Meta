package orderedmap

// Adapted from https://github.com/dvyukov/go-fuzz-corpus/blob/c42c1b2/json/json.go

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func FuzzRoundTripYAML(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, testCase := range []struct {
			name        string
			constructor func() any
			// should be a function that asserts that 2 objects of the type returned by constructor are equal
			equalityAssertion func(*testing.T, any, any) bool
		}{
			{
				name:              "with a string -> string map",
				constructor:       func() any { return &OrderedMap[string, string]{} },
				equalityAssertion: assertOrderedMapsEqual[string, string],
			},
			{
				name:              "with a string -> int map",
				constructor:       func() any { return &OrderedMap[string, int]{} },
				equalityAssertion: assertOrderedMapsEqual[string, int],
			},
			{
				name:              "with a string -> any map",
				constructor:       func() any { return &OrderedMap[string, any]{} },
				equalityAssertion: assertYAMLAnyOrderedMapsEqual,
			},
			{
				name:              "with a struct with map fields",
				constructor:       func() any { return new(testFuzzStruct) },
				equalityAssertion: assertTestFuzzStructEqual,
			},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				v1 := testCase.constructor()
				if yaml.Unmarshal(data, v1) != nil {
					return
				}
				t.Log(data)
				t.Log(v1)

				yamlData, err := yaml.Marshal(v1)
				require.NoError(t, err)
				t.Log(string(yamlData))

				v2 := testCase.constructor()
				err = yaml.Unmarshal(yamlData, v2)
				if err != nil {
					t.Log(string(yamlData))
					t.Fatal(err)
				}

				if !assert.True(t, testCase.equalityAssertion(t, v1, v2), "failed with input data %q", string(data)) {
					// look at that what the standard lib does with regular map, to help with debugging

					var m1 map[string]any
					require.NoError(t, yaml.Unmarshal(data, &m1))

					mapJsonData, err := yaml.Marshal(m1)
					require.NoError(t, err)

					var m2 map[string]any
					require.NoError(t, yaml.Unmarshal(mapJsonData, &m2))

					t.Logf("initial data = %s", string(data))
					t.Logf("unmarshalled map = %v", m1)
					t.Logf("re-marshalled from map = %s", string(mapJsonData))
					t.Logf("re-marshalled from test obj = %s", string(yamlData))
					t.Logf("re-unmarshalled map = %s", m2)
				}
			})
		}
	})
}

// YAML resolves scalars such as "08" and 8 to different Go types. For an
// `any` value, compare canonical YAML so numeric normalization is accepted
// while string/number distinctions still fail.
func assertYAMLAnyOrderedMapsEqual(t *testing.T, v1, v2 any) bool {
	om1, ok1 := v1.(*OrderedMap[string, any])
	om2, ok2 := v2.(*OrderedMap[string, any])
	if !assert.True(t, ok1, "v1 not an orderedmap") ||
		!assert.True(t, ok2, "v2 not an orderedmap") {
		return false
	}

	success := assert.Equal(t, om1.Len(), om2.Len(), "om1 and om2 have different lengths: %d vs %d", om1.Len(), om2.Len())
	for pair1, pair2, index := om1.Oldest(), om2.Oldest(), 0; pair1 != nil && pair2 != nil; pair1, pair2, index = pair1.Next(), pair2.Next(), index+1 {
		success = assert.Equal(t, pair1.Key, pair2.Key, "different keys at position %d: %v vs %v", index, pair1.Key, pair2.Key) && success

		value1, err := yaml.Marshal(pair1.Value)
		require.NoError(t, err)
		value2, err := yaml.Marshal(pair2.Value)
		require.NoError(t, err)
		success = assert.True(t, bytes.Equal(bytes.TrimSpace(value1), bytes.TrimSpace(value2)),
			"different YAML values at position %d: %s vs %s", index, bytes.TrimSpace(value1), bytes.TrimSpace(value2)) && success
	}
	return success
}
