package resell

import (
	"bytes"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strings"
)

func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
func (c *Client) validateAccount() error {
	if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func allocationOperation(resource, action, method string) core.Operation {
	op := core.Operation{ID: "projects.v2." + resource + "." + action, Service: "projects", APIGeneration: "v2", Method: method, Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401}
	}
	if method == "DELETE" {
		op.Success = []int{204}
	}
	if action == "get" || action == "delete" {
		op.ErrorCodes = map[int]types.Code{400: types.NotFound}
	}
	if resource == "subnets_projects" {
		op.Rejected = []int{401}
	}
	return op
}
func int32Value(value *types.Integer, min int64) bool {
	if value == nil {
		return false
	}
	n, ok := new(big.Int).SetString(value.String(), 10)
	return ok && n.IsInt64() && n.Int64() >= min && n.Int64() <= 2147483647
}

// decodeAllocations accepts each shape independently confirmed by the public schema
// or official SDK. It extracts trustworthy IDs before decoding other fields.
func decodeAllocations[T any](singular, plural, kind, project string, numeric bool, assign func(*T, *types.ResourceRef), validate func(T) error) core.Decoder[[]T] {
	return func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[[]T], err error) {
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
			return out, types.NewError(types.ProtocolError)
		}
		var rows []json.RawMessage
		one, hasOne := envelope[singular]
		many, hasMany := envelope[plural]
		if singular == plural {
			hasOne = false
		}
		if hasOne && hasMany {
			return out, types.NewError(types.ProtocolError)
		}
		if hasOne {
			if bytes.Equal(bytes.TrimSpace(one), []byte("null")) {
				return out, types.NewError(types.ProtocolError)
			}
			rows = []json.RawMessage{one}
		} else if hasMany {
			if json.Unmarshal(many, &rows) != nil || rows == nil {
				return out, types.NewError(types.ProtocolError)
			}
		} else {
			return out, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		remember := func(code types.Code) {
			if err == nil {
				err = types.NewError(code)
			}
		}
		for _, row := range rows {
			var fields map[string]json.RawMessage
			if json.Unmarshal(row, &fields) != nil || fields == nil {
				remember(types.ProtocolError)
				continue
			}
			var id string
			if numeric {
				var n types.Integer
				if json.Unmarshal(fields["id"], &n) != nil {
					remember(types.ProtocolError)
					continue
				}
				id = n.String()
			} else if json.Unmarshal(fields["id"], &id) != nil || id == "" {
				remember(types.ProtocolError)
				continue
			}
			if p, ok := fields["project_id"]; ok && string(p) != "null" {
				var actual string
				if json.Unmarshal(p, &actual) != nil {
					remember(types.ProtocolError)
					continue
				}
				if project != "" && actual != project {
					remember(types.ScopeMismatch)
					continue
				}
			}
			// Only a row with independently trustworthy scope and identity can
			// reserve an ID. A foreign row cannot hide a later valid allocation.
			if seen[id] {
				remember(types.ProtocolError)
				continue
			}
			seen[id] = true
			ref := core.Ref(endpoint, "projects", kind, id)
			out.Refs = append(out.Refs, *ref)
		}
		if err != nil {
			return out, err
		}
		out.Data = make([]T, 0, len(rows))
		for i, row := range rows {
			var value T
			if json.Unmarshal(row, &value) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if e := validate(value); e != nil {
				return out, e
			}
			assign(&value, &out.Refs[i])
			out.Data = append(out.Data, value)
		}
		return out, nil
	}
}

func (c *Client) validateAllocation() error {
	if c.scope.Region != "" || c.scope.Pool != "" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Client) allocationOperation(resource, action, method string) core.Operation {
	op := allocationOperation(resource, action, method)
	if c.scope.ProjectID != "" {
		op.Auth = types.IAMProject
		op.RequireProject = true
	}
	return op
}
