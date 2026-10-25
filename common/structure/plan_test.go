package structure

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeStructFromMapReference is the per-decode walk this package used before
// the field plan existed. It is kept so the plan can be checked against what it
// replaced, field by field and error message by error message.
func (d *Decoder) decodeStructFromMapReference(name string, dataVal, val reflect.Value) error {
	dataValType := dataVal.Type()
	if kind := dataValType.Key().Kind(); kind != reflect.String && kind != reflect.Interface {
		return fmt.Errorf(
			"'%s' needs a map with string keys, has '%s' keys",
			name, dataValType.Key().Kind())
	}

	dataValKeys := make(map[reflect.Value]struct{})
	dataValKeysUnused := make(map[any]struct{})
	for _, dataValKey := range dataVal.MapKeys() {
		dataValKeys[dataValKey] = struct{}{}
		dataValKeysUnused[dataValKey.Interface()] = struct{}{}
	}

	targetValKeysUnused := make(map[any]struct{})
	errors := make([]string, 0)

	structs := make([]reflect.Value, 1, 5)
	structs[0] = val

	type field struct {
		field reflect.StructField
		val   reflect.Value
	}

	var remainField *field

	var fields []field
	for len(structs) > 0 {
		structVal := structs[0]
		structs = structs[1:]

		structType := structVal.Type()

		for i := 0; i < structType.NumField(); i++ {
			fieldType := structType.Field(i)
			fieldVal := structVal.Field(i)
			if fieldVal.Kind() == reflect.Ptr && fieldVal.Elem().Kind() == reflect.Struct {
				fieldVal = fieldVal.Elem()
			}

			squash := fieldVal.Kind() == reflect.Struct && fieldType.Anonymous
			remain := false

			tagParts := strings.Split(fieldType.Tag.Get(d.option.TagName), ",")
			for _, tag := range tagParts[1:] {
				if tag == "squash" {
					squash = true
					break
				}

				if tag == "remain" {
					remain = true
					break
				}
			}

			if squash {
				if fieldVal.Kind() != reflect.Struct {
					errors = append(errors,
						fmt.Errorf("%s: unsupported type for squash: %s", fieldType.Name, fieldVal.Kind()).Error())
				} else {
					structs = append(structs, fieldVal)
				}
				continue
			}

			if remain {
				remainField = &field{fieldType, fieldVal}
			} else {
				fields = append(fields, field{fieldType, fieldVal})
			}
		}
	}

	for _, f := range fields {
		field, fieldValue := f.field, f.val
		fieldName := field.Name

		tagParts := strings.Split(field.Tag.Get(d.option.TagName), ",")
		tagValue := tagParts[0]
		if tagValue != "" {
			fieldName = tagValue
		}

		if tagValue == "-" {
			continue
		}

		omitempty := false
		for _, tag := range tagParts[1:] {
			if tag == "omitempty" {
				omitempty = true
			}
		}

		rawMapKey := reflect.ValueOf(fieldName)
		rawMapVal := dataVal.MapIndex(rawMapKey)
		if !rawMapVal.IsValid() {
			if d.option.KeyReplacer != nil {
				fieldName = d.option.KeyReplacer.Replace(fieldName)
			}
			for dataValKey := range dataValKeys {
				mK, ok := dataValKey.Interface().(string)
				if !ok {
					continue
				}
				if d.option.KeyReplacer != nil {
					mK = d.option.KeyReplacer.Replace(mK)
				}

				if strings.EqualFold(mK, fieldName) {
					rawMapKey = dataValKey
					rawMapVal = dataVal.MapIndex(dataValKey)
					break
				}
			}

			if !rawMapVal.IsValid() {
				if !omitempty {
					targetValKeysUnused[fieldName] = struct{}{}
				}
				continue
			}
		}

		delete(dataValKeysUnused, rawMapKey.Interface())

		if !fieldValue.IsValid() {
			panic("field is not valid")
		}

		if !fieldValue.CanSet() {
			continue
		}

		if name != "" {
			fieldName = name + "." + fieldName
		}

		if err := d.decode(fieldName, rawMapVal.Interface(), fieldValue); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if remainField != nil && len(dataValKeysUnused) > 0 {
		remain := map[interface{}]interface{}{}
		for key := range dataValKeysUnused {
			remain[key] = dataVal.MapIndex(reflect.ValueOf(key)).Interface()
		}

		if err := d.decodeMap(name, remain, remainField.val); err != nil {
			errors = append(errors, err.Error())
		}

		dataValKeysUnused = nil
	}

	if len(targetValKeysUnused) > 0 {
		keys := make([]string, 0, len(targetValKeysUnused))
		for rawKey := range targetValKeysUnused {
			keys = append(keys, rawKey.(string))
		}
		sort.Strings(keys)

		err := fmt.Errorf("'%s' has unset fields: %s", name, strings.Join(keys, ", "))
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, ","))
	}

	return nil
}

type planBase struct {
	Alpha string `test:"alpha"`
	Beta  int    `test:"beta,omitempty"`
}

type planOption struct {
	planBase
	Name    string            `test:"name"`
	Port    int               `test:"port"`
	UDP     bool              `test:"udp,omitempty"`
	Network string            `test:"network,omitempty"`
	Headers map[string]string `test:"headers,omitempty"`
	Rest    map[string]any    `test:",remain"`
	Ignored string            `test:"-"`
}

type planUnexported struct {
	Value    string `test:"value"`
	internal int
}

type planInner struct {
	Bar string `test:"bar"`
}

type planNested struct {
	Opt planInner `test:"opt"`
}

type planReplaced struct {
	Value string `test:"some_key"`
}

type planBadSquash struct {
	Number int `test:"number,squash"`
}

type planNumTarget struct {
	Num  num  `test:"num"`
	NumP *num `test:"num_p"`
}

var foldDecoder = NewDecoder(Option{TagName: "test", KeyReplacer: DefaultKeyReplacer})

func TestPlanOfCachesPerTypeAndTag(t *testing.T) {
	typ := reflect.TypeOf(planBase{})
	first := planOf(typ, "test")
	assert.Same(t, first, planOf(typ, "test"))
	assert.NotSame(t, first, planOf(typ, "proxy"))
}

func TestPlanReadsEachDecoderOwnTag(t *testing.T) {
	type tagged struct {
		Value string `test:"test-value" proxy:"proxy-value"`
	}
	source := map[string]any{"test-value": "one", "proxy-value": "two"}

	fromTest := &tagged{}
	require.NoError(t, decoder.Decode(source, fromTest))
	assert.Equal(t, "one", fromTest.Value)

	fromProxy := &tagged{}
	require.NoError(t, NewDecoder(Option{TagName: "proxy"}).Decode(source, fromProxy))
	assert.Equal(t, "two", fromProxy.Value)
}

// TestPlanDecodesWhatTheFieldWalkDecoded runs both decoders over the same
// shapes and compares the value that comes out and the error text, so the plan
// is pinned to the walk it replaced rather than to a hand written expectation.
func TestPlanDecodesWhatTheFieldWalkDecoded(t *testing.T) {
	cases := []struct {
		name    string
		typ     reflect.Type
		source  any
		decoder *Decoder
	}{
		{name: "base", typ: reflect.TypeOf(planBase{}), source: map[string]any{"alpha": "a", "beta": 7, "extra": true}},
		{name: "base-missing", typ: reflect.TypeOf(planBase{}), source: map[string]any{"beta": 7}},
		{name: "base-nil-value", typ: reflect.TypeOf(planBase{}), source: map[string]any{"alpha": nil, "beta": 7}},
		{name: "base-type-error", typ: reflect.TypeOf(planBase{}), source: map[string]any{"alpha": []int{1}, "beta": 7}},
		{
			name:   "option",
			typ:    reflect.TypeOf(planOption{}),
			source: map[string]any{"alpha": "a", "name": "node", "port": 443, "udp": true, "headers": map[string]string{"host": "example.com"}, "extra": map[string]any{"path": "/ws"}},
		},
		{
			name:   "option-without-extra",
			typ:    reflect.TypeOf(planOption{}),
			source: map[string]any{"name": "node", "port": 443},
		},
		{
			name:   "option-missing-name",
			typ:    reflect.TypeOf(planOption{}),
			source: map[string]any{"port": 443},
		},
		{
			name:   "option-ignored-key",
			typ:    reflect.TypeOf(planOption{}),
			source: map[string]any{"name": "node", "port": 443, "ignored": "left alone"},
		},
		{name: "unexported-missing", typ: reflect.TypeOf(planUnexported{}), source: map[string]any{}},
		{name: "unexported-present", typ: reflect.TypeOf(planUnexported{}), source: map[string]any{"value": "v", "internal": 3}},
		{name: "nested", typ: reflect.TypeOf(planNested{}), source: map[string]any{"opt": map[string]any{"bar": "b"}}},
		{name: "nested-nil", typ: reflect.TypeOf(planNested{}), source: map[string]any{"opt": map[string]any{"bar": nil}}},
		{name: "nested-error", typ: reflect.TypeOf(planNested{}), source: map[string]any{"opt": map[string]any{"bar": []int{1}}}},
		{name: "bad-squash", typ: reflect.TypeOf(planBadSquash{}), source: map[string]any{"number": 1}},
		{name: "text-unmarshaller", typ: reflect.TypeOf(planNumTarget{}), source: map[string]any{"num": "255", "num_p": "127"}},
		{name: "text-unmarshaller-error", typ: reflect.TypeOf(planNumTarget{}), source: map[string]any{"num": "abc", "num_p": "127"}},
		{name: "string-map", typ: reflect.TypeOf(planBase{}), source: map[string]string{"alpha": "a", "beta": "7"}},
		{name: "interface-keys", typ: reflect.TypeOf(planBase{}), source: map[any]any{"alpha": "a", "beta": 7}},
		{name: "interface-keys-fold", typ: reflect.TypeOf(planBase{}), source: map[any]any{"ALPHA": "a", "beta": 7}},
		{name: "replacer", typ: reflect.TypeOf(planReplaced{}), source: map[string]any{"some-key": "hit"}, decoder: foldDecoder},
		{name: "replacer-fold", typ: reflect.TypeOf(planReplaced{}), source: map[string]any{"SOME_KEY": "hit"}, decoder: foldDecoder},
		{name: "case-fold", typ: reflect.TypeOf(planReplaced{}), source: map[string]any{"Some_Key": "hit"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.decoder
			if d == nil {
				d = decoder
			}
			dataVal := reflect.Indirect(reflect.ValueOf(tc.source))

			reference := reflect.New(tc.typ)
			referenceErr := d.decodeStructFromMapReference("", dataVal, reference.Elem())

			planned := reflect.New(tc.typ)
			plannedErr := d.decodeStructFromMap("", dataVal, planned.Elem())

			require.Equal(t, referenceErr != nil, plannedErr != nil, "reference error: %v, plan error: %v", referenceErr, plannedErr)
			if referenceErr != nil {
				require.Equal(t, referenceErr.Error(), plannedErr.Error())
				return
			}
			require.Equal(t, reference.Elem().Interface(), planned.Elem().Interface())
		})
	}
}

// An embedded struct pointer is followed by allocating it when it is nil, so a
// squashed struct is walked the same way whether or not it was allocated first.
func TestPlanFollowsEmbeddedStructPointer(t *testing.T) {
	type Inner struct {
		Value string `test:"value"`
	}
	type Outer struct {
		*Inner
	}
	target := &Outer{}
	require.NoError(t, decoder.Decode(map[string]any{"value": "hit"}, target))
	require.NotNil(t, target.Inner)
	assert.Equal(t, "hit", target.Inner.Value)
}

// A struct that embeds itself is walked as a plain field instead of being
// followed forever.
func TestPlanStopsAtSelfEmbeddingStruct(t *testing.T) {
	type Node struct {
		*Node
		Value string `test:"value"`
	}
	target := &Node{}
	err := decoder.Decode(map[string]any{"value": "hit"}, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has unset fields: Node")
}

func TestPlanDecodesConcurrently(t *testing.T) {
	source := map[string]any{"alpha": "a", "name": "node", "port": 443, "udp": true, "headers": map[string]string{"host": "example.com"}}
	typ := reflect.TypeOf(planOption{})
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := 0; i < 64; i++ {
				target := reflect.New(typ).Interface()
				assert.NoError(t, decoder.Decode(source, target))
			}
		}()
	}
	group.Wait()
}

// A pointer field that is already allocated is decoded into what it points at.
func TestPlanDecodesIntoAllocatedPointerField(t *testing.T) {
	type Inner struct {
		Value string `test:"value"`
	}
	target := &struct {
		Field *Inner `test:"field"`
	}{Field: &Inner{}}
	require.NoError(t, decoder.Decode(map[string]any{"field": map[string]any{"value": "hit"}}, target))
	require.NotNil(t, target.Field)
	assert.Equal(t, "hit", target.Field.Value)
}

type planBenchOption struct {
	planBase
	planInner
	Name              string            `test:"name"`
	Server            string            `test:"server"`
	Port              int               `test:"port"`
	Type              string            `test:"type"`
	Password          string            `test:"password"`
	Cipher            string            `test:"cipher"`
	UDP               bool              `test:"udp,omitempty"`
	SkipCertVerify    bool              `test:"skip-cert-verify,omitempty"`
	SNI               string            `test:"sni,omitempty"`
	Network           string            `test:"network,omitempty"`
	WsOpts            map[string]any    `test:"ws-opts,omitempty"`
	Headers           map[string]string `test:"headers,omitempty"`
	ClientFingerprint string            `test:"client-fingerprint,omitempty"`
}

func planBenchSource() map[string]any {
	return map[string]any{
		"alpha":              "a",
		"beta":               7,
		"bar":                "b",
		"name":               "node",
		"server":             "10.0.0.1",
		"port":               443,
		"type":               "ss",
		"password":           "password",
		"cipher":             "aes-128-gcm",
		"udp":                true,
		"skip-cert-verify":   true,
		"sni":                "example.com",
		"network":            "ws",
		"ws-opts":            map[string]any{"path": "/ws", "headers": map[string]any{"Host": "example.com"}},
		"headers":            map[string]string{"host": "example.com"},
		"client-fingerprint": "chrome",
	}
}

func benchmarkDecodeStruct(b *testing.B, decode func(*Decoder, string, reflect.Value, reflect.Value) error) {
	b.Helper()
	typ := reflect.TypeOf(planBenchOption{})
	source := reflect.ValueOf(planBenchSource())
	target := reflect.New(typ).Elem()
	zero := reflect.Zero(typ)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target.Set(zero)
		if err := decode(decoder, "", source, target); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeStructPlan(b *testing.B) {
	benchmarkDecodeStruct(b, (*Decoder).decodeStructFromMap)
}

func BenchmarkDecodeStructReference(b *testing.B) {
	benchmarkDecodeStruct(b, (*Decoder).decodeStructFromMapReference)
}
