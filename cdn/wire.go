package cdn

import (
	"encoding/json"
	"reflect"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/types"
)

// wireBody is private to compiled typed mutations. Ordinary DTO JSON redacts;
// this boundary alone reveals explicitly supplied protected request strings.
func wireBody(value interface{ Validate() error }) ([]byte, error) {
	if value.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	encoded, e := wireValue(reflect.ValueOf(value))
	if e != nil {
		return nil, e
	}
	if e := validateWireModel(value, encoded); e != nil {
		return nil, e
	}
	body, e := json.Marshal(encoded)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return body, nil
}
func wireValue(v reflect.Value) (any, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, types.NewError(types.InvalidArgument)
		}
		return wireValue(v.Elem())
	}
	if v.CanInterface() {
		switch value := v.Interface().(type) {
		case types.ProtectedValue:
			raw := value.Reveal()
			if !utf8.Valid(raw) {
				return nil, types.NewError(types.InvalidArgument)
			}
			return string(raw), nil
		case types.Integer:
			n, e := types.ParseInteger(value.String())
			if e != nil {
				return nil, types.NewError(types.InvalidArgument)
			}
			return n, nil
		}
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return nil, types.NewError(types.InvalidArgument)
		}
		return v.String(), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Slice:
		if v.IsNil() {
			return nil, types.NewError(types.InvalidArgument)
		}
		out := []any{}
		for i := 0; i < v.Len(); i++ {
			value, e := wireValue(v.Index(i))
			if e != nil {
				return nil, e
			}
			out = append(out, value)
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() {
			return nil, types.NewError(types.InvalidArgument)
		}
		out := map[string]any{}
		for _, key := range v.MapKeys() {
			if key.Kind() != reflect.String || !utf8.ValidString(key.String()) {
				return nil, types.NewError(types.InvalidArgument)
			}
			value, e := wireValue(v.MapIndex(key))
			if e != nil {
				return nil, e
			}
			out[key.String()] = value
		}
		return out, nil
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		if t.PkgPath() != "github.com/festy23/selectel-sdk/cdn" {
			return nil, types.NewError(types.InvalidArgument)
		}
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			value := v.Field(i)
			if field.Name == "Extra" || field.Name == "Ref" {
				if !value.IsZero() && !(value.Kind() == reflect.Map && value.Len() == 0) {
					return nil, types.NewError(types.InvalidArgument)
				}
				continue
			}
			if value.Kind() == reflect.Pointer && value.IsNil() {
				continue
			}
			key := field.Tag.Get("json")
			for j, c := range key {
				if c == ',' {
					key = key[:j]
					break
				}
			}
			if key == "" || key == "-" {
				return nil, types.NewError(types.InvalidArgument)
			}
			encoded, e := wireValue(value)
			if e != nil {
				return nil, e
			}
			out[key] = encoded
		}
		return out, nil
	}
	return nil, types.NewError(types.InvalidArgument)
}
