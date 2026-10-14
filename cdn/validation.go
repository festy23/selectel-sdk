package cdn

import (
	_ "embed"
	"encoding/json"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/festy23/selectel-sdk/types"
)

//go:embed request_schema_generated.json
var requestSchemaRaw []byte

type wireSchema struct {
	Ref        string                `json:"$ref"`
	AllOf      []wireSchema          `json:"allOf"`
	Type       string                `json:"type"`
	Properties map[string]wireSchema `json:"properties"`
	Required   []string              `json:"required"`
	Items      *wireSchema           `json:"items"`
	Additional *wireSchema           `json:"additionalProperties"`
	Enum       []string              `json:"enum"`
	Minimum    *json.Number          `json:"minimum"`
	Maximum    *json.Number          `json:"maximum"`
	Pattern    string                `json:"pattern"`
}

var requestSchemaState struct {
	sync.Once
	defs map[string]wireSchema
	err  error
}

func validateWireModel(model any, wire any) error {
	requestSchemaState.Do(func() { requestSchemaState.err = json.Unmarshal(requestSchemaRaw, &requestSchemaState.defs) })
	if requestSchemaState.err != nil {
		return types.NewError(types.CapabilityUnavailable)
	}
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	schema, ok := requestSchemaState.defs["models."+t.Name()]
	if !ok {
		return types.NewError(types.InvalidArgument)
	}
	return validateWireSchema(schema, wire, 0)
}
func validateWireSchema(s wireSchema, value any, depth int) error {
	bad := func() error { return types.NewError(types.InvalidArgument) }
	if depth > 64 {
		return bad()
	}
	if s.Ref != "" {
		ref, ok := requestSchemaState.defs[strings.TrimPrefix(s.Ref, "#/definitions/")]
		if !ok {
			return bad()
		}
		return validateWireSchema(ref, value, depth+1)
	}
	if len(s.AllOf) > 0 {
		for _, part := range s.AllOf {
			if e := validateWireSchema(part, value, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	switch s.Type {
	case "object":
		fields, ok := value.(map[string]any)
		if !ok {
			return bad()
		}
		for _, key := range s.Required {
			if _, ok := fields[key]; !ok {
				return bad()
			}
		}
		for key, v := range fields {
			property, ok := s.Properties[key]
			if !ok {
				if s.Additional == nil {
					return bad()
				}
				property = *s.Additional
			}
			if e := validateWireSchema(property, v, depth+1); e != nil {
				return e
			}
		}
	case "array":
		values, ok := value.([]any)
		if !ok || s.Items == nil {
			return bad()
		}
		for _, v := range values {
			if e := validateWireSchema(*s.Items, v, depth+1); e != nil {
				return e
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return bad()
		}
		if len(s.Enum) > 0 {
			found := false
			for _, v := range s.Enum {
				found = found || v == text
			}
			if !found {
				return bad()
			}
		}
		if s.Pattern != "" {
			pattern, e := regexp.Compile(s.Pattern)
			if e != nil || !pattern.MatchString(text) {
				return bad()
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return bad()
		}
	case "integer":
		integer, ok := value.(types.Integer)
		if !ok {
			return bad()
		}
		number, ok := new(big.Int).SetString(integer.String(), 10)
		if !ok {
			return bad()
		}
		for i, bound := range []*json.Number{s.Minimum, s.Maximum} {
			if bound == nil {
				continue
			}
			limit, ok := new(big.Int).SetString(bound.String(), 10)
			if !ok {
				return bad()
			}
			comparison := number.Cmp(limit)
			if (i == 0 && comparison < 0) || (i == 1 && comparison > 0) {
				return bad()
			}
		}
	default:
		return bad()
	}
	return nil
}
