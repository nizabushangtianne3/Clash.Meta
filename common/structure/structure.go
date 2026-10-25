package structure

// references: https://github.com/mitchellh/mapstructure

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Option is the configuration that is used to create a new decoder
type Option struct {
	TagName          string
	WeaklyTypedInput bool
	KeyReplacer      *strings.Replacer
}

var DefaultKeyReplacer = strings.NewReplacer("_", "-")

// Decoder is the core of structure
type Decoder struct {
	option *Option
}

// NewDecoder return a Decoder by Option
func NewDecoder(option Option) *Decoder {
	if option.TagName == "" {
		option.TagName = "structure"
	}
	return &Decoder{option: &option}
}

// Decode transform a map[string]any to a struct
func (d *Decoder) Decode(src map[string]any, dst any) error {
	if reflect.TypeOf(dst).Kind() != reflect.Ptr {
		return fmt.Errorf("decode must recive a ptr struct")
	}
	return d.decode("", src, reflect.ValueOf(dst).Elem())
}

// isNil returns true if the input is nil or a typed nil pointer.
func isNil(input any) bool {
	if input == nil {
		return true
	}
	val := reflect.ValueOf(input)
	return val.Kind() == reflect.Pointer && val.IsNil()
}

func (d *Decoder) decode(name string, data any, val reflect.Value) error {
	if isNil(data) {
		// If the data is nil, then we don't set anything
		// Maybe we should set to zero value?
		return nil
	}
	if !reflect.ValueOf(data).IsValid() {
		// If the input value is invalid, then we just set the value
		// to be the zero value.
		val.Set(reflect.Zero(val.Type()))
		return nil
	}
	for {
		kind := val.Kind()
		if kind == reflect.Pointer && val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		if ok, err := d.decodeTextUnmarshaller(name, data, val); ok {
			return err
		}
		switch {
		case isInt(kind):
			return d.decodeInt(name, data, val)
		case isUint(kind):
			return d.decodeUint(name, data, val)
		case isFloat(kind):
			return d.decodeFloat(name, data, val)
		}
		switch kind {
		case reflect.Pointer:
			val = val.Elem()
			continue
		case reflect.String:
			return d.decodeString(name, data, val)
		case reflect.Bool:
			return d.decodeBool(name, data, val)
		case reflect.Slice:
			return d.decodeSlice(name, data, val)
		case reflect.Map:
			return d.decodeMap(name, data, val)
		case reflect.Interface:
			return d.setInterface(name, data, val)
		case reflect.Struct:
			return d.decodeStruct(name, data, val)
		default:
			return fmt.Errorf("type %s not support", val.Kind().String())
		}
	}
}

func isInt(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

func isUint(kind reflect.Kind) bool {
	switch kind {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func isFloat(kind reflect.Kind) bool {
	switch kind {
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func (d *Decoder) decodeInt(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	kind := dataVal.Kind()
	switch {
	case isInt(kind):
		val.SetInt(dataVal.Int())
	case isUint(kind) && d.option.WeaklyTypedInput:
		val.SetInt(int64(dataVal.Uint()))
	case isFloat(kind) && d.option.WeaklyTypedInput:
		val.SetInt(int64(dataVal.Float()))
	case kind == reflect.String && d.option.WeaklyTypedInput:
		var i int64
		i, err = strconv.ParseInt(dataVal.String(), 0, val.Type().Bits())
		if err == nil {
			val.SetInt(i)
		} else {
			err = fmt.Errorf("cannot parse '%s' as int: %s", name, err)
		}
	default:
		err = fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s'",
			name, val.Type(), dataVal.Type(),
		)
	}
	return err
}

func (d *Decoder) decodeUint(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	kind := dataVal.Kind()
	switch {
	case isUint(kind):
		val.SetUint(dataVal.Uint())
	case isInt(kind) && d.option.WeaklyTypedInput:
		val.SetUint(uint64(dataVal.Int()))
	case isFloat(kind) && d.option.WeaklyTypedInput:
		val.SetUint(uint64(dataVal.Float()))
	case kind == reflect.String && d.option.WeaklyTypedInput:
		var i uint64
		i, err = strconv.ParseUint(dataVal.String(), 0, val.Type().Bits())
		if err == nil {
			val.SetUint(i)
		} else {
			err = fmt.Errorf("cannot parse '%s' as int: %s", name, err)
		}
	default:
		err = fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s'",
			name, val.Type(), dataVal.Type(),
		)
	}
	return err
}

func (d *Decoder) decodeFloat(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	kind := dataVal.Kind()
	switch {
	case isFloat(kind):
		val.SetFloat(dataVal.Float())
	case isUint(kind):
		val.SetFloat(float64(dataVal.Uint()))
	case isInt(kind) && d.option.WeaklyTypedInput:
		val.SetFloat(float64(dataVal.Int()))
	case kind == reflect.String && d.option.WeaklyTypedInput:
		var i float64
		i, err = strconv.ParseFloat(dataVal.String(), val.Type().Bits())
		if err == nil {
			val.SetFloat(i)
		} else {
			err = fmt.Errorf("cannot parse '%s' as int: %s", name, err)
		}
	default:
		err = fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s'",
			name, val.Type(), dataVal.Type(),
		)
	}
	return err
}

func (d *Decoder) decodeString(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	kind := dataVal.Kind()
	switch {
	case kind == reflect.String:
		val.SetString(dataVal.String())
	case isInt(kind) && d.option.WeaklyTypedInput:
		val.SetString(strconv.FormatInt(dataVal.Int(), 10))
	case isUint(kind) && d.option.WeaklyTypedInput:
		val.SetString(strconv.FormatUint(dataVal.Uint(), 10))
	case isFloat(kind) && d.option.WeaklyTypedInput:
		val.SetString(strconv.FormatFloat(dataVal.Float(), 'E', -1, dataVal.Type().Bits()))
	default:
		err = fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s'",
			name, val.Type(), dataVal.Type(),
		)
	}
	return err
}

func (d *Decoder) decodeBool(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	kind := dataVal.Kind()
	switch {
	case kind == reflect.Bool:
		val.SetBool(dataVal.Bool())
	case isInt(kind) && d.option.WeaklyTypedInput:
		val.SetBool(dataVal.Int() != 0)
	case isUint(kind) && d.option.WeaklyTypedInput:
		val.SetBool(dataVal.Uint() != 0)
	default:
		err = fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s'",
			name, val.Type(), dataVal.Type(),
		)
	}
	return err
}

func (d *Decoder) decodeSlice(name string, data any, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	valType := val.Type()
	valElemType := valType.Elem()

	if dataVal.Kind() == reflect.String && valElemType.Kind() == reflect.Uint8 { // from encoding/json
		s := []byte(dataVal.String())
		b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
		n, err := base64.StdEncoding.Decode(b, s)
		if err != nil {
			return fmt.Errorf("try decode '%s' by base64 error: %w", name, err)
		}
		val.SetBytes(b[:n])
		return nil
	}

	if dataVal.Kind() != reflect.Slice {
		return fmt.Errorf("'%s' is not a slice", name)
	}

	valSlice := val
	// make a new slice with cap(val)==cap(dataVal)
	// the caller can determine whether the original configuration contains this item by judging whether the value is nil.
	valSlice = reflect.MakeSlice(valType, 0, dataVal.Len())
	for i := 0; i < dataVal.Len(); i++ {
		currentData := dataVal.Index(i).Interface()
		for valSlice.Len() <= i {
			valSlice = reflect.Append(valSlice, reflect.Zero(valElemType))
		}
		fieldName := fmt.Sprintf("%s[%d]", name, i)
		if currentData == nil {
			// in weakly type mode, null will convert to zero value
			if d.option.WeaklyTypedInput {
				continue
			}
			// in non-weakly type mode, null will convert to nil if element's zero value is nil, otherwise return an error
			if elemKind := valElemType.Kind(); elemKind == reflect.Map || elemKind == reflect.Slice {
				continue
			}
			return fmt.Errorf("'%s' can not be null", fieldName)
		}
		currentField := valSlice.Index(i)
		if err := d.decode(fieldName, currentData, currentField); err != nil {
			return err
		}
	}

	val.Set(valSlice)
	return nil
}

func (d *Decoder) decodeMap(name string, data any, val reflect.Value) error {
	valType := val.Type()
	valKeyType := valType.Key()
	valElemType := valType.Elem()

	valMap := val

	if valMap.IsNil() {
		mapType := reflect.MapOf(valKeyType, valElemType)
		valMap = reflect.MakeMap(mapType)
	}

	dataVal := reflect.Indirect(reflect.ValueOf(data))
	if dataVal.Kind() != reflect.Map {
		return fmt.Errorf("'%s' expected a map, got '%s'", name, dataVal.Kind())
	}

	return d.decodeMapFromMap(name, dataVal, val, valMap)
}

func (d *Decoder) decodeMapFromMap(name string, dataVal reflect.Value, val reflect.Value, valMap reflect.Value) error {
	valType := val.Type()
	valKeyType := valType.Key()
	valElemType := valType.Elem()

	errors := make([]string, 0)

	if dataVal.Len() == 0 {
		if dataVal.IsNil() {
			if !val.IsNil() {
				val.Set(dataVal)
			}
		} else {
			val.Set(valMap)
		}

		return nil
	}

	for _, k := range dataVal.MapKeys() {
		fieldName := fmt.Sprintf("%s[%s]", name, k)

		currentKey := reflect.Indirect(reflect.New(valKeyType))
		if err := d.decode(fieldName, k.Interface(), currentKey); err != nil {
			errors = append(errors, err.Error())
			continue
		}

		v := dataVal.MapIndex(k).Interface()
		if v == nil {
			errors = append(errors, fmt.Sprintf("filed %s invalid", fieldName))
			continue
		}

		currentVal := reflect.Indirect(reflect.New(valElemType))
		if err := d.decode(fieldName, v, currentVal); err != nil {
			errors = append(errors, err.Error())
			continue
		}

		valMap.SetMapIndex(currentKey, currentVal)
	}

	val.Set(valMap)

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, ","))
	}

	return nil
}

func (d *Decoder) decodeStruct(name string, data any, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))

	// If the type of the value to write to and the data match directly,
	// then we just set it directly instead of recursing into the structure.
	if dataVal.Type() == val.Type() {
		val.Set(dataVal)
		return nil
	}

	dataValKind := dataVal.Kind()
	switch dataValKind {
	case reflect.Map:
		return d.decodeStructFromMap(name, dataVal, val)
	default:
		return fmt.Errorf("'%s' expected a map, got '%s'", name, dataVal.Kind())
	}
}

// A struct's tags are fixed by its type, so the decoder compiles them once per
// type and tag name instead of splitting them again for every field of every
// value it decodes.
type plannedField struct {
	index     int
	name      string
	omitempty bool
	remain    bool
	squash    bool
	pointer   bool
	badSquash string
	children  []plannedField
}

type structPlan struct {
	fields []plannedField
	remain bool
}

type planKey struct {
	typ reflect.Type
	tag string
}

var structPlans sync.Map

func planOf(typ reflect.Type, tag string) *structPlan {
	key := planKey{typ, tag}
	if cached, ok := structPlans.Load(key); ok {
		return cached.(*structPlan)
	}
	plan := compileStructPlan(typ, tag)
	structPlans.Store(key, plan)
	return plan
}

// compileStructPlan walks a struct the way decoding used to walk it: a level's
// fields keep their declaration order and the structs squashed into it are
// queued behind them, breadth first.
func compileStructPlan(structType reflect.Type, tag string) *structPlan {
	type scope struct {
		owner *plannedField
		typ   reflect.Type
		stack []reflect.Type
	}
	var fields []plannedField
	queue := []scope{{nil, structType, []reflect.Type{structType}}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		level := make([]plannedField, 0, current.typ.NumField())
		var squashed []int
		for i := 0; i < current.typ.NumField(); i++ {
			fieldType := current.typ.Field(i)
			field := plannedField{index: i, name: fieldType.Name}

			fieldStructType := fieldType.Type
			pointer := fieldStructType.Kind() == reflect.Pointer && fieldStructType.Elem().Kind() == reflect.Struct
			if pointer {
				fieldStructType = fieldStructType.Elem()
			}

			name, tagSquash, tagRemain, omitempty := parseFieldTag(fieldType.Tag.Get(tag))
			field.omitempty = omitempty

			squash := tagSquash || (fieldStructType.Kind() == reflect.Struct && fieldType.Anonymous)
			if squash && typeOnStack(current.stack, fieldStructType) {
				// a struct embedding itself would be walked forever
				squash = false
			}
			if squash {
				if fieldStructType.Kind() != reflect.Struct {
					field.badSquash = fmt.Sprintf("%s: unsupported type for squash: %s", fieldType.Name, fieldType.Type.Kind())
					level = append(level, field)
					continue
				}
				field.squash = true
				field.pointer = pointer
				level = append(level, field)
				squashed = append(squashed, len(level)-1)
				continue
			}
			if tagRemain {
				field.remain = true
				level = append(level, field)
				continue
			}
			if name == "-" {
				continue
			}
			if name != "" {
				field.name = name
			}
			level = append(level, field)
		}

		if current.owner == nil {
			fields = level
		} else {
			current.owner.children = level
		}
		for _, index := range squashed {
			field := &level[index]
			embedded := current.typ.Field(field.index).Type
			if field.pointer {
				embedded = embedded.Elem()
			}
			stack := make([]reflect.Type, len(current.stack)+1)
			copy(stack, current.stack)
			stack[len(current.stack)] = embedded
			queue = append(queue, scope{field, embedded, stack})
		}
	}

	plan := &structPlan{fields: fields}
	for i := range fields {
		if fields[i].remain {
			plan.remain = true
		}
	}
	return plan
}

func typeOnStack(stack []reflect.Type, typ reflect.Type) bool {
	for _, embedded := range stack {
		if embedded == typ {
			return true
		}
	}
	return false
}

// parseFieldTag reads a struct tag the way the per-field loop did: the first
// part names the field, "omitempty" anywhere after it makes the field optional,
// and the first "squash" or "remain" after it decides how the field is walked.
func parseFieldTag(tag string) (name string, squash, remain, omitempty bool) {
	name = tag
	rest := ""
	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		name, rest = tag[:comma], tag[comma+1:]
	}
	for rest != "" {
		part := rest
		if comma := strings.IndexByte(rest, ','); comma >= 0 {
			part, rest = rest[:comma], rest[comma+1:]
		} else {
			rest = ""
		}
		switch {
		case part == "omitempty":
			omitempty = true
		case part == "squash" && !remain:
			squash = true
		case part == "remain" && !squash:
			remain = true
		}
	}
	return
}

// mapSource reads the map a struct is decoded from. A plain map[string]any,
// which is what the config parser builds, is read directly; every other map
// shape goes through reflection.
type mapSource struct {
	native map[string]any
	value  reflect.Value
	keys   []string
	keyed  bool
}

func (s *mapSource) get(key string) (any, string, bool) {
	if s.native != nil {
		value, ok := s.native[key]
		if !ok {
			return nil, key, false
		}
		return value, key, true
	}
	raw := s.value.MapIndex(reflect.ValueOf(key))
	if !raw.IsValid() {
		return nil, key, false
	}
	return raw.Interface(), key, true
}

func (s *mapSource) keyList() []string {
	if !s.keyed {
		s.keyed = true
		if s.native != nil {
			s.keys = make([]string, 0, len(s.native))
			for key := range s.native {
				s.keys = append(s.keys, key)
			}
			return s.keys
		}
		for _, key := range s.value.MapKeys() {
			if text, ok := key.Interface().(string); ok {
				s.keys = append(s.keys, text)
			}
		}
	}
	return s.keys
}

func (s *mapSource) getFold(key string, replacer *strings.Replacer) (any, string, bool) {
	for _, sourceKey := range s.keyList() {
		compare := sourceKey
		if replacer != nil {
			compare = replacer.Replace(compare)
		}
		if strings.EqualFold(compare, key) {
			return s.get(sourceKey)
		}
	}
	return nil, "", false
}

type structDecodeState struct {
	decoder    *Decoder
	source     mapSource
	planErrors []string
	errors     []string
	missing    []string
	unused     map[string]struct{}
	remain     reflect.Value
}

func (d *Decoder) decodeStructFromMap(name string, dataVal, val reflect.Value) error {
	dataValType := dataVal.Type()
	if kind := dataValType.Key().Kind(); kind != reflect.String && kind != reflect.Interface {
		return fmt.Errorf(
			"'%s' needs a map with string keys, has '%s' keys",
			name, dataValType.Key().Kind())
	}

	plan := planOf(val.Type(), d.option.TagName)

	state := &structDecodeState{decoder: d, source: mapSource{value: dataVal}}
	if native, ok := dataVal.Interface().(map[string]any); ok {
		state.source.native = native
	}
	if plan.remain {
		keys := state.source.keyList()
		state.unused = make(map[string]struct{}, len(keys))
		for _, key := range keys {
			state.unused[key] = struct{}{}
		}
	}

	state.decodeFields(name, plan.fields, val)

	if state.remain.IsValid() && len(state.unused) > 0 {
		remain := make(map[any]any, len(state.unused))
		for key := range state.unused {
			value, _, _ := state.source.get(key)
			remain[key] = value
		}
		if err := state.decoder.decodeMap(name, remain, state.remain); err != nil {
			state.errors = append(state.errors, err.Error())
		}
	}
	if len(state.missing) > 0 {
		sort.Strings(state.missing)
		state.errors = append(state.errors,
			fmt.Errorf("'%s' has unset fields: %s", name, strings.Join(dedupeSorted(state.missing), ", ")).Error())
	}
	if len(state.planErrors) > 0 {
		state.errors = append(state.planErrors, state.errors...)
	}
	if len(state.errors) > 0 {
		return fmt.Errorf("%s", strings.Join(state.errors, ","))
	}
	return nil
}

func dedupeSorted(values []string) []string {
	if len(values) < 2 {
		return values
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func (s *structDecodeState) decodeFields(name string, fields []plannedField, structVal reflect.Value) {
	for i := range fields {
		field := &fields[i]
		switch {
		case field.badSquash != "":
			s.planErrors = append(s.planErrors, field.badSquash)
		case field.squash:
			child := structVal.Field(field.index)
			if field.pointer {
				if child.IsNil() {
					if !child.CanSet() {
						continue
					}
					child.Set(reflect.New(child.Type().Elem()))
				}
				child = child.Elem()
			}
			s.decodeFields(name, field.children, child)
		case field.remain:
			s.remain = structVal.Field(field.index)
		default:
			s.decodeField(name, field, structVal)
		}
	}
}

func (s *structDecodeState) decodeField(name string, field *plannedField, structVal reflect.Value) {
	fieldName := field.name
	fieldValue := structVal.Field(field.index)

	value, sourceKey, ok := s.source.get(fieldName)
	if !ok {
		lookup := fieldName
		if s.decoder.option.KeyReplacer != nil {
			lookup = s.decoder.option.KeyReplacer.Replace(lookup)
		}
		value, sourceKey, ok = s.source.getFold(lookup, s.decoder.option.KeyReplacer)
		if !ok {
			if !field.omitempty {
				s.missing = append(s.missing, lookup)
			}
			return
		}
		fieldName = lookup
	}

	if s.unused != nil {
		delete(s.unused, sourceKey)
	}
	if !fieldValue.CanSet() {
		return
	}
	if name != "" {
		fieldName = name + "." + fieldName
	}
	if err := s.decoder.decode(fieldName, value, fieldValue); err != nil {
		s.errors = append(s.errors, err.Error())
	}
}

func (d *Decoder) setInterface(name string, data any, val reflect.Value) (err error) {
	dataVal := reflect.ValueOf(data)
	val.Set(dataVal)
	return nil
}

func (d *Decoder) decodeTextUnmarshaller(name string, data any, val reflect.Value) (bool, error) {
	if !val.CanAddr() {
		return false, nil
	}
	valAddr := val.Addr()
	if !valAddr.CanInterface() {
		return false, nil
	}
	unmarshaller, ok := valAddr.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return false, nil
	}
	var str string
	if err := d.decodeString(name, data, reflect.Indirect(reflect.ValueOf(&str))); err != nil {
		return false, err
	}
	if err := unmarshaller.UnmarshalText([]byte(str)); err != nil {
		return true, fmt.Errorf("cannot parse '%s' as %s: %s", name, val.Type(), err)
	}
	return true, nil
}
