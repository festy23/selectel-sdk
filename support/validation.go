package support

import (
	_ "embed"
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed ticket_request_schema_generated.json
var ticketSchemaRaw []byte

type wireSchema struct {
	Ref                                      string                `json:"$ref"`
	Type                                     string                `json:"type"`
	Format                                   string                `json:"format"`
	Nullable                                 bool                  `json:"nullable"`
	Properties                               map[string]wireSchema `json:"properties"`
	Required                                 []string              `json:"required"`
	Items                                    *wireSchema           `json:"items"`
	Additional                               *wireSchema           `json:"additionalProperties"`
	Enum                                     []json.RawMessage     `json:"enum"`
	Minimum, Maximum                         *json.Number
	MinLength, MaxLength, MinItems, MaxItems *uint
}

var ticketSchemaState struct {
	sync.Once
	defs map[string]wireSchema
	err  error
}

func validateWireModel(model any, wire any) error {
	ticketSchemaState.Do(func() { ticketSchemaState.err = json.Unmarshal(ticketSchemaRaw, &ticketSchemaState.defs) })
	if ticketSchemaState.err != nil {
		return types.NewError(types.CapabilityUnavailable)
	}
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	schema, ok := ticketSchemaState.defs[typ.Name()]
	if !ok {
		return types.NewError(types.InvalidArgument)
	}
	return validateWireSchema(schema, wire, 0)
}
func validateWireSchema(schema wireSchema, value any, depth int) error {
	bad := func() error { return types.NewError(types.InvalidArgument) }
	if depth > 64 {
		return bad()
	}
	if value == nil {
		if schema.Nullable {
			return nil
		}
		return bad()
	}
	if schema.Ref != "" {
		target, ok := ticketSchemaState.defs[strings.TrimPrefix(schema.Ref, "#/components/schemas/")]
		if !ok {
			return bad()
		}
		if schema.Nullable {
			target.Nullable = true
		}
		return validateWireSchema(target, value, depth+1)
	}
	if len(schema.Enum) > 0 {
		raw, e := json.Marshal(value)
		if e != nil {
			return bad()
		}
		found := false
		for _, item := range schema.Enum {
			if string(raw) == string(item) {
				found = true
			}
		}
		if !found {
			return bad()
		}
	}
	switch schema.Type {
	case "object":
		fields, ok := value.(map[string]any)
		if !ok {
			return bad()
		}
		for _, key := range schema.Required {
			if _, ok := fields[key]; !ok {
				return bad()
			}
		}
		for key, item := range fields {
			field, ok := schema.Properties[key]
			if !ok {
				if schema.Additional == nil {
					continue
				}
				field = *schema.Additional
			}
			if e := validateWireSchema(field, item, depth+1); e != nil {
				return e
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok || schema.Items == nil {
			return bad()
		}
		n := uint(len(items))
		if schema.MinItems != nil && n < *schema.MinItems || schema.MaxItems != nil && n > *schema.MaxItems {
			return bad()
		}
		for _, item := range items {
			if e := validateWireSchema(*schema.Items, item, depth+1); e != nil {
				return e
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok || !utf8.ValidString(text) {
			return bad()
		}
		n := uint(utf8.RuneCountInString(text))
		if schema.MinLength != nil && n < *schema.MinLength || schema.MaxLength != nil && n > *schema.MaxLength {
			return bad()
		}
		if schema.Format == "uuid" && !fileUUID.MatchString(text) {
			return bad()
		}
	case "integer":
		raw, e := json.Marshal(value)
		if e != nil {
			return bad()
		}
		integer, ok := new(big.Int).SetString(string(raw), 10)
		if !ok {
			return bad()
		}
		if schema.Format == "int32" && (!integer.IsInt64() || integer.Int64() < -2147483648 || integer.Int64() > 2147483647) {
			return bad()
		}
		for _, bound := range []struct {
			value   *json.Number
			minimum bool
		}{{schema.Minimum, true}, {schema.Maximum, false}} {
			if bound.value != nil {
				n, ok := new(big.Int).SetString(bound.value.String(), 10)
				if !ok {
					return bad()
				}
				comparison := integer.Cmp(n)
				if bound.minimum && comparison < 0 || !bound.minimum && comparison > 0 {
					return bad()
				}
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return bad()
		}
	}
	return nil
}
