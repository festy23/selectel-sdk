package dedicated

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func scanResultRefs(raw json.RawMessage, scope types.Scope, endpoint types.EndpointBinding, kind string, expected *types.ResourceRef) ([]types.ResourceRef, error) {
	var rows []json.RawMessage
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, nil
	}
	if trim[0] == '[' {
		if json.Unmarshal(trim, &rows) != nil {
			return nil, types.NewError(types.ProtocolError)
		}
	} else {
		rows = []json.RawMessage{trim}
	}
	refs := []types.ResourceRef{}
	seen := map[string]bool{}
	var first error
	for _, row := range rows {
		var fields map[string]json.RawMessage
		if json.Unmarshal(row, &fields) != nil || fields == nil {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			continue
		}
		key := "uuid"
		if kind == "task" {
			key = "task_id"
		}
		var id *string
		if _, present := fields[key]; !present {
			continue
		}
		if json.Unmarshal(fields[key], &id) != nil || id == nil || *id == "" {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			continue
		}
		if project, present := fields["project_uuid"]; present && scope.ProjectID != "" {
			var value *string
			if json.Unmarshal(project, &value) != nil || value == nil || *value != scope.ProjectID {
				if first == nil {
					first = types.NewError(types.ScopeMismatch)
				}
				continue
			}
		}
		if expected != nil && expected.Kind == kind && *id != expected.ID {
			if first == nil {
				first = types.NewError(types.ScopeMismatch)
			}
			continue
		}
		if seen[*id] {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			continue
		}
		seen[*id] = true
		ref := core.Ref(endpoint, "dedicated", kind, *id)
		if expected != nil && expected.ID == *id && expected.Kind == kind {
			copy := cloneRef(*expected)
			ref = &copy
		}
		if kind == "resource" {
			if rawPrimary, present := fields["primary_uuid"]; present {
				var primary *string
				if json.Unmarshal(rawPrimary, &primary) != nil || primary == nil {
					if first == nil {
						first = types.NewError(types.ProtocolError)
					}
				} else {
					if ref.Parent != nil && *primary != ref.Parent.ID {
						if first == nil {
							first = types.NewError(types.ScopeMismatch)
						}
						continue
					}
				}
			}
		}
		refs = append(refs, *ref)
	}
	return refs, first
}
func bindResultRefs(value any, refs []types.ResourceRef) {
	v := reflect.ValueOf(value).Elem()
	result := v.FieldByName("Result")
	if !result.IsValid() {
		return
	}
	if result.Kind() == reflect.Pointer {
		if result.IsNil() {
			return
		}
		result = result.Elem()
	}
	bind := func(row reflect.Value) {
		if row.Kind() != reflect.Struct {
			return
		}
		field := row.FieldByName("Ref")
		if !field.IsValid() || !field.CanSet() {
			return
		}
		id := row.FieldByName("UUID")
		if !id.IsValid() {
			id = row.FieldByName("TaskID")
		}
		if !id.IsValid() || id.IsNil() {
			return
		}
		for _, ref := range refs {
			if ref.ID == id.Elem().String() {
				copy := cloneRef(ref)
				field.Set(reflect.ValueOf(&copy))
				return
			}
		}
	}
	if result.Kind() == reflect.Slice {
		for i := 0; i < result.Len(); i++ {
			bind(result.Index(i))
		}
	} else {
		bind(result)
	}
}

func cloneRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		parent := cloneRef(*ref.Parent)
		ref.Parent = &parent
	}
	return ref
}
func strictExpected(method string, ref *types.ResourceRef) *types.ResourceRef {
	if method == "POST" {
		return nil
	}
	return ref
}
func requestProject(spec any) (string, bool) {
	v := reflect.ValueOf(spec)
	if !v.IsValid() {
		return "", false
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	f := v.FieldByName("ProjectUUID")
	if !f.IsValid() || f.IsNil() {
		return "", false
	}
	return f.Elem().String(), true
}
func verifyTransfer(raw []byte, id, target string) error {
	var identity struct{ UUID, ProjectUUID *string }
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return types.NewError(types.ProtocolError)
	}
	if field, ok := fields["uuid"]; ok {
		if json.Unmarshal(field, &identity.UUID) != nil {
			return types.NewError(types.ProtocolError)
		}
		if identity.UUID == nil || *identity.UUID != id {
			return types.NewError(types.ScopeMismatch)
		}
	}
	if field, ok := fields["project_uuid"]; ok {
		if json.Unmarshal(field, &identity.ProjectUUID) != nil {
			return types.NewError(types.ProtocolError)
		}
		if identity.ProjectUUID == nil || *identity.ProjectUUID != target {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func bindDirect(value any, fields map[string]json.RawMessage, ref *types.ResourceRef) error {
	if ref == nil {
		return types.NewError(types.ProtocolError)
	}
	if raw, present := fields["uuid"]; present {
		var id *string
		if json.Unmarshal(raw, &id) != nil {
			return types.NewError(types.ProtocolError)
		}
		if id == nil || *id != ref.ID {
			return types.NewError(types.ScopeMismatch)
		}
	}
	v := reflect.ValueOf(value).Elem()
	field := v.FieldByName("Ref")
	if field.IsValid() && field.CanSet() {
		copy := cloneRef(*ref)
		field.Set(reflect.ValueOf(&copy))
	}
	return nil
}
