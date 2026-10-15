package filestorage

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

// wireValue reveals protected inputs only at the private request boundary.
// Ordinary JSON/formatting of public DTOs remains redacted.
func wireValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return wireValue(v.Elem())
	}
	if v.CanInterface() {
		switch x := v.Interface().(type) {
		case types.Nullable[string]:
			return nullableWireValue(x)
		case types.Nullable[bool]:
			return nullableWireValue(x)
		case types.Nullable[types.Integer]:
			return nullableWireValue(x)
		case types.Nullable[types.Number]:
			return nullableWireValue(x)
		case types.Nullable[types.ProtectedValue]:
			return nullableWireValue(x)
		case types.Nullable[types.ProtectedJSON]:
			return nullableWireValue(x)
		case time.Time:
			return x.UTC(), nil
		case types.ProtectedJSON:
			raw := x.Reveal()
			if !json.Valid(raw) {
				return nil, types.NewError(types.InvalidArgument)
			}
			return json.RawMessage(raw), nil
		case types.ProtectedValue:
			raw := x.Reveal()
			if !utf8.Valid(raw) {
				return nil, types.NewError(types.InvalidArgument)
			}
			return string(raw), nil
		case json.Marshaler:
			if v.Type().PkgPath() == "github.com/festy23/selectel-sdk/types" && strings.HasPrefix(v.Type().Name(), "Nullable[") {
				return nil, types.NewError(types.InvalidArgument)
			}
			return x, nil
		}
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")
			key := tag[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = field.Name
			}
			fv := v.Field(i)
			if len(tag) > 1 && ((tag[1] == "omitzero" && fv.IsZero()) || (tag[1] == "omitempty" && fv.Kind() == reflect.Pointer && fv.IsNil())) {
				continue
			}
			value, err := wireValue(fv)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		return out, nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil, nil
		}
		out := make([]any, v.Len())
		for i := range out {
			value, err := wireValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			value, err := wireValue(iter.Value())
			if err != nil {
				return nil, err
			}
			key := iter.Key().String()
			if !utf8.ValidString(key) {
				return nil, types.NewError(types.InvalidArgument)
			}
			out[key] = value
		}
		return out, nil
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	return v.Interface(), nil
}
func wireBody(value any) ([]byte, error) {
	v, err := wireValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}

func nullableWireValue[T any](input types.Nullable[T]) (any, error) {
	value, valid := input.Value()
	if !valid {
		return nil, nil
	}
	return wireValue(reflect.ValueOf(value))
}
