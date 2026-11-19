package sentry

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Decode fills target (a pointer) from JSON the way the types of this package
// read the protocol: a value of the wrong type leaves its field at the zero
// value, numbers and booleans fill string fields as their text, list elements
// and map values of the wrong shape are dropped. It reports whether data had
// the shape of target, e.g. an object (not null) for a struct.
func Decode(data []byte, target any) bool {
	value := reflect.ValueOf(target).Elem()

	if json.Unmarshal(data, target) == nil {
		return value.Kind() != reflect.Struct || jsontext.Value(data).Kind() == jsontext.KindBeginObject
	}

	value.SetZero()

	decoder := &lenientDecoder{
		Decoder: jsontext.NewDecoder(bytes.NewBuffer(data), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)),
	}

	if decoder.decode(value) && !decoder.failed {
		if _, err := decoder.ReadToken(); err == io.EOF {
			return true
		}
	}

	value.SetZero()

	return false
}

// lenientDecoder walks one token stream, so reading around a wrong-typed field
// costs one more pass over the payload, however deep the field.
type lenientDecoder struct {
	*jsontext.Decoder
	failed bool
}

// valuesList is a list that may come wrapped as {"values": [...]}.
type valuesList interface{ valuesList() }

func (*ExceptionList) valuesList()  {}
func (*BreadcrumbList) valuesList() {}

// decode reads exactly one value into value and reports whether it had its shape.
func (decoder *lenientDecoder) decode(value reflect.Value) bool {
	if decoder.failed {
		return false
	}

	target := value.Addr().Interface()

	if _, ok := target.(valuesList); ok {
		return decoder.decodeValuesList(value)
	}

	if unmarshaler, ok := target.(json.Unmarshaler); ok {
		raw, err := decoder.ReadValue()

		if err != nil {
			decoder.failed = true
			return false
		}

		return unmarshaler.UnmarshalJSON(raw) == nil
	}

	kind := decoder.PeekKind()

	switch value.Kind() {
	case reflect.Pointer:
		if kind == jsontext.KindNull {
			return decoder.token()
		}

		element := reflect.New(value.Type().Elem())

		if !decoder.decode(element.Elem()) {
			return false
		}

		value.Set(element)
	case reflect.Struct:
		if kind != jsontext.KindBeginObject {
			return decoder.skip()
		}

		fields := structFields(value.Type())

		decoder.token()

		for decoder.more(jsontext.KindEndObject) {
			if index, ok := fields.find(decoder.name()); ok {
				decoder.decode(value.Field(index))
			} else {
				decoder.skip()
			}
		}

		return decoder.token()
	case reflect.Slice:
		return decoder.decodeSlice(value)
	case reflect.Map:
		if kind == jsontext.KindNull {
			return decoder.token()
		}

		if kind != jsontext.KindBeginObject || value.Type().Key().Kind() != reflect.String {
			return decoder.skip()
		}

		result := reflect.MakeMap(value.Type())

		decoder.token()

		for decoder.more(jsontext.KindEndObject) {
			key := reflect.ValueOf(decoder.name()).Convert(value.Type().Key())
			element := reflect.New(value.Type().Elem()).Elem()

			if decoder.decode(element) {
				result.SetMapIndex(key, element)
			}
		}

		value.Set(result)

		return decoder.token()
	case reflect.String:
		text, ok := decoder.scalar()

		if ok && kind != jsontext.KindNull {
			value.SetString(text)
		}

		return ok
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		text, ok := decoder.scalar()
		number, err := strconv.ParseFloat(text, 64)

		if !ok || err != nil || math.IsNaN(number) || math.Abs(number) >= math.MaxInt64 || value.OverflowInt(int64(number)) {
			return false
		}

		value.SetInt(int64(number))
	case reflect.Float32, reflect.Float64:
		text, ok := decoder.scalar()
		number, err := strconv.ParseFloat(text, 64)

		if !ok || err != nil || value.OverflowFloat(number) {
			return false
		}

		value.SetFloat(number)
	default:
		// Booleans only as JSON booleans, as ingest's compaction reads in_app.
		raw, err := decoder.ReadValue()

		if err != nil {
			decoder.failed = true
			return false
		}

		return json.Unmarshal(raw, target) == nil
	}

	return true
}

func (decoder *lenientDecoder) decodeSlice(value reflect.Value) bool {
	switch decoder.PeekKind() {
	case jsontext.KindNull:
		return decoder.token()
	case jsontext.KindBeginArray:
	default:
		return decoder.skip()
	}

	result := reflect.MakeSlice(value.Type(), 0, 0)
	element := reflect.New(value.Type().Elem()).Elem()

	decoder.token()

	for decoder.more(jsontext.KindEndArray) {
		element.SetZero()

		if decoder.decode(element) {
			result = reflect.Append(result, element)
		}
	}

	value.Set(result)

	return decoder.token()
}

func (decoder *lenientDecoder) decodeValuesList(value reflect.Value) bool {
	if decoder.PeekKind() != jsontext.KindBeginObject {
		return decoder.decodeSlice(value)
	}

	decoder.token()

	for decoder.more(jsontext.KindEndObject) {
		if decoder.name() == "values" {
			decoder.decodeSlice(value)
		} else {
			decoder.skip()
		}
	}

	return decoder.token()
}

// scalar reads a string, number, boolean or null as text; objects and arrays
// are skipped, as Relay's LenientString drops them.
func (decoder *lenientDecoder) scalar() (string, bool) {
	switch decoder.PeekKind() {
	case jsontext.KindBeginObject, jsontext.KindBeginArray:
		return "", decoder.skip()
	}

	token, err := decoder.ReadToken()

	if err != nil {
		decoder.failed = true
		return "", false
	}

	return token.String(), true
}

func (decoder *lenientDecoder) name() string {
	token, err := decoder.ReadToken()

	if err != nil {
		decoder.failed = true
		return ""
	}

	return token.String()
}

// token reads a delimiter or null; it reports false on a read error.
func (decoder *lenientDecoder) token() bool {
	if _, err := decoder.ReadToken(); err != nil {
		decoder.failed = true
	}

	return !decoder.failed
}

// skip passes over a value of the wrong shape; it always reports false.
func (decoder *lenientDecoder) skip() bool {
	if decoder.SkipValue() != nil {
		decoder.failed = true
	}

	return false
}

func (decoder *lenientDecoder) more(end jsontext.Kind) bool {
	kind := decoder.PeekKind()

	return !decoder.failed && kind != end && kind != jsontext.KindInvalid
}

// fieldNames maps JSON names to field indexes of a struct type.
type fieldNames map[string]int

var structFieldCache sync.Map

func structFields(structType reflect.Type) fieldNames {
	if cached, ok := structFieldCache.Load(structType); ok {
		return cached.(fieldNames)
	}

	fields := fieldNames{}

	for index := range structType.NumField() {
		field := structType.Field(index)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")

		if !field.IsExported() || name == "-" {
			continue
		}

		if name == "" {
			name = field.Name
		}

		fields[name] = index
	}

	structFieldCache.Store(structType, fields)

	return fields
}

// find matches names like encoding/json: exactly, else ignoring case.
func (fields fieldNames) find(name string) (int, bool) {
	if index, ok := fields[name]; ok {
		return index, true
	}

	for fieldName, index := range fields {
		if strings.EqualFold(fieldName, name) {
			return index, true
		}
	}

	return 0, false
}
