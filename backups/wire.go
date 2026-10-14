package backups

import (
	"encoding/json"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/types"
)

// wireValue reveals protected inputs only at the private HTTP boundary.
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
		case types.ProtectedValue:
			raw := x.Reveal()
			if !utf8.Valid(raw) {
				return nil, types.NewError(types.InvalidArgument)
			}
			return string(raw), nil
		}
		method := v.MethodByName("Value")
		if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 2 && method.Type().Out(1).Kind() == reflect.Bool {
			values := method.Call(nil)
			if !values[1].Bool() {
				return nil, nil
			}
			return wireValue(values[0])
		}
		if _, ok := v.Interface().(json.Marshaler); ok {
			return v.Interface(), nil
		}
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "-" || key == "ref" {
				continue
			}
			field := v.Field(i)
			if (field.Kind() == reflect.Pointer || field.Kind() == reflect.Slice || field.Kind() == reflect.Map) && field.IsNil() {
				continue
			}
			value, e := wireValue(field)
			if e != nil {
				return nil, e
			}
			out[key] = value
		}
		return out, nil
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			value, e := wireValue(v.Index(i))
			if e != nil {
				return nil, e
			}
			out[i] = value
		}
		return out, nil
	default:
		return v.Interface(), nil
	}
}
func requestBody(value any) ([]byte, error) {
	wire, e := wireValue(reflect.ValueOf(value))
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(wire)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}
func inputError(v interface{ Validate() error }) error {
	if v.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
