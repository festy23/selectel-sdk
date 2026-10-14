package loadbalancers

import (
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Input DTOs are generator-owned. Required fields have no omitempty tag; optional
// zero/false/empty values retain pointer presence. Providers enforce permissions
// and plugin-specific enums; validation never infers them from resource ownership.
func validateInput(v any) error { return validateValue(reflect.ValueOf(v), "") }
func validateValue(v reflect.Value, field string) error {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return validateValue(v.Elem(), field)
	}
	if v.CanInterface() {
		if n, ok := v.Interface().(types.Integer); ok {
			x, valid := new(big.Int).SetString(n.String(), 10)
			if !valid {
				return types.NewError(types.InvalidArgument)
			}
			min, max := "", ""
			switch field {
			case "protocol_port", "monitor_port":
				min = "1"
				max = "65535"
			case "weight":
				min = "0"
				max = "256"
			case "max_retries", "max_retries_down":
				min = "1"
				max = "10"
			case "delay", "timeout", "timeout_client_data", "timeout_member_connect", "timeout_member_data", "timeout_tcp_inspect":
				min = "0"
			case "connection_limit":
				min = "-1"
			}
			for _, bound := range []struct {
				raw   string
				lower bool
			}{{min, true}, {max, false}} {
				if bound.raw != "" {
					b, _ := new(big.Int).SetString(bound.raw, 10)
					cmp := x.Cmp(b)
					if bound.lower && cmp < 0 || !bound.lower && cmp > 0 {
						return types.NewError(types.InvalidArgument)
					}
				}
			}
			return nil
		}
		if n, ok := v.Interface().(types.Number); ok {
			if n.String() == "" {
				return types.NewError(types.InvalidArgument)
			}
			return nil
		}
	}
	if v.Kind() == reflect.Struct {
		// Nullable exposes its explicit null/value boundary through Value().
		if method := v.MethodByName("Value"); method.IsValid() {
			result := method.Call(nil)
			if len(result) == 2 && result[1].Kind() == reflect.Bool {
				if !result[1].Bool() {
					return nil
				}
				return validateValue(result[0], field)
			}
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			parts := strings.Split(f.Tag.Get("json"), ",")
			if parts[0] == "-" {
				continue
			}
			fv := v.Field(i)
			if fv.Kind() == reflect.String && len(parts) == 1 && fv.String() == "" {
				return types.NewError(types.InvalidArgument)
			}
			if e := validateValue(fv, parts[0]); e != nil {
				return e
			}
		}
	} else if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		for i := 0; i < v.Len(); i++ {
			if e := validateValue(v.Index(i), field); e != nil {
				return e
			}
		}
	} else if v.Kind() == reflect.Map {
		it := v.MapRange()
		for it.Next() {
			if e := validateValue(it.Key(), field); e != nil {
				return e
			}
			if e := validateValue(it.Value(), field); e != nil {
				return e
			}
		}
	} else if v.Kind() == reflect.String {
		if !utf8.ValidString(v.String()) {
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
