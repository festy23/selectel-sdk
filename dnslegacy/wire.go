package dnslegacy

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/festy23/selectel-sdk/types"
)

func wireBody(value any) ([]byte, error) {
	mapped, err := wireValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(mapped)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}
func wireValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return wireValue(v.Elem())
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		return wireValue(v.Elem())
	}
	if v.CanInterface() {
		switch value := v.Interface().(type) {
		case types.ProtectedValue:
			return string(value.Reveal()), nil
		case types.Integer:
			if _, err := types.ParseInteger(value.String()); err != nil {
				return nil, err
			}
			raw, _ := value.MarshalJSON()
			return json.RawMessage(raw), nil
		}
		if union, ok := v.Interface().(interface{ wireVariant() (any, error) }); ok {
			value, err := union.wireVariant()
			if err != nil {
				return nil, err
			}
			return wireValue(reflect.ValueOf(value))
		}
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "-" || name == "ref" {
				continue
			}
			if name == "" {
				return nil, types.NewError(types.InvalidArgument)
			}
			value := v.Field(i)
			if len(parts) > 1 && (parts[1] == "omitempty" || parts[1] == "omitzero") && value.IsZero() {
				continue
			}
			mapped, err := wireValue(value)
			if err != nil {
				return nil, err
			}
			out[name] = mapped
		}
		return out, nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil, nil
		}
		out := make([]any, v.Len())
		for i := range out {
			mapped, err := wireValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = mapped
		}
		return out, nil
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, types.NewError(types.InvalidArgument)
		}
		if v.IsNil() {
			return nil, nil
		}
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			mapped, err := wireValue(iter.Value())
			if err != nil {
				return nil, err
			}
			out[iter.Key().String()] = mapped
		}
		return out, nil
	case reflect.String, reflect.Bool:
		return v.Interface(), nil
	default:
		return nil, types.NewError(types.InvalidArgument)
	}
}
