package mks

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

type service struct {
	runtime *core.Runtime
	scope   types.Scope
}
type validatable interface{ Validate() error }

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
func requestBody(field string, value any) ([]byte, error) {
	v, err := wireValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}
	if field != "" {
		v = map[string]any{field: v}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}
func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
func resourcePath(ref types.ResourceRef) string {
	switch ref.Kind {
	case "cluster":
		return "/clusters/" + segment(ref.ID)
	case "nodegroup":
		if ref.Parent != nil {
			return resourcePath(*ref.Parent) + "/nodegroups/" + segment(ref.ID)
		}
	case "node":
		if ref.Parent != nil {
			return resourcePath(*ref.Parent) + "/nodes/" + segment(ref.ID)
		}
	case "task":
		if ref.Parent != nil {
			return resourcePath(*ref.Parent) + "/tasks/" + segment(ref.ID)
		}
	}
	return "/invalid-ref"
}
func validateRef(ref types.ResourceRef, kind string, scope types.Scope) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Scope != scope || ref.Service != "mks" || ref.APIGeneration != "v2" {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != kind {
		return types.NewError(types.InvalidArgument)
	}
	if kind == "cluster" {
		if ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	parentKind := "cluster"
	if kind == "node" {
		parentKind = "nodegroup"
	}
	if ref.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != ref.Parent.EndpointID || ref.EndpointURL != ref.Parent.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return validateRef(*ref.Parent, parentKind, scope)
}
func pinned(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func invoke[T any](s service, ctx context.Context, id, method, path string, success int, ref *types.ResourceRef, body []byte, query url.Values, validation error, decode core.Decoder[T]) (types.Result[T], error) {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	op := core.Operation{ID: id, Service: "mks", APIGeneration: "v2", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{success}}
	if effect == "mutation" {
		op.Rejected = []int{401, 403}
		if ref != nil {
			op.Rejected = append(op.Rejected, 404)
		}
		if strings.Contains(id, "nodegroups") || id == "mks.v2.clusters.update" {
			op.Rejected = append(op.Rejected, 400, 409)
		}
		if strings.Contains(id, "registries") {
			op.Rejected = append(op.Rejected, 400)
		}
	}
	op, bindErr := s.runtime.BindDocumented(op, s.scope)
	in := core.Request{Scope: s.scope, Path: path, Body: body, Query: query, Validate: func() error {
		if validation != nil {
			return validation
		}
		return bindErr
	}}
	if ref != nil {
		in.Refs = []types.ResourceRef{*ref}
		in.PinnedEndpoint = pinned(*ref)
	}
	return core.DoJSON(s.runtime, ctx, op, in, decode)
}
func empty(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	if len(strings.TrimSpace(string(raw))) != 0 {
		return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
	}
	return core.Decoded[struct{}]{}, nil
}
func decodeObject[T validatable](field, kind, expected string, parent *types.ResourceRef, assign func(*T, types.ResourceRef), check func(T) error) core.Decoder[T] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[T], error) {
		if field != "" {
			var envelope map[string]json.RawMessage
			if json.Unmarshal(raw, &envelope) != nil {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			raw = envelope[field]
		}
		var identity struct {
			ID          string  `json:"id"`
			ProjectID   *string `json:"project_id"`
			Pool        *string `json:"pool"`
			ClusterID   *string `json:"cluster_id"`
			NodegroupID *string `json:"nodegroup_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == "" {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		if expected != "" && identity.ID != expected {
			return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
		}
		if (identity.ProjectID != nil && *identity.ProjectID != endpoint.Scope.ProjectID) || (identity.Pool != nil && *identity.Pool != endpoint.Scope.Pool) || (parent != nil && ((kind == "nodegroup" || kind == "task") && identity.ClusterID != nil && *identity.ClusterID != parent.ID || kind == "node" && identity.NodegroupID != nil && *identity.NodegroupID != parent.ID)) {
			return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
		}
		ref := *core.Ref(endpoint, "mks", kind, identity.ID)
		ref.Parent = parent
		out := core.Decoded[T]{Refs: []types.ResourceRef{ref}}
		var value T
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if err := value.Validate(); err != nil {
			return out, err
		}
		if err := check(value); err != nil {
			return core.Decoded[T]{}, err
		}
		assign(&value, ref)
		out.Data = value
		return out, nil
	}
}
func inputError(value validatable) error {
	if value.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}

func ownedRef(ref types.ResourceRef) types.ResourceRef {
	raw, _ := json.Marshal(ref)
	var out types.ResourceRef
	_ = json.Unmarshal(raw, &out)
	return out
}
func handle(s service, ref types.ResourceRef, receipt *types.Receipt, read operations.Read, goals map[string]operations.Goal) (*operations.Handle, error) {
	if receipt != nil {
		capability, err := s.runtime.Catalog().Describe(receipt.OperationID)
		if err != nil {
			return nil, err
		}
		allowed := map[string]operations.Goal{}
		for _, id := range capability.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	return operations.Bind(s.runtime, ref, receipt, read, goals)
}
