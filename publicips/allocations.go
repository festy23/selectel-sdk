package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
)

type Allocations struct {
	runtime *core.Runtime
	scope   types.Scope
}

func BindAllocations(r *core.Runtime, s types.Scope) *Allocations {
	return &Allocations{runtime: r, scope: s}
}

type AllocationCreateRequest struct {
	ProjectID string
	Specs     []AllocationSpec
}

func (a *Allocations) op(action, method string) core.Operation {
	mode := types.IAMAccount
	if a.scope.ProjectID != "" {
		mode = types.IAMProject
	}
	op := core.Operation{ID: "projects.v2.floatingips." + action, Service: "projects", APIGeneration: "v2", Auth: mode, Method: method, Effect: "read", EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403}
	}
	if method == "POST" {
		op.ID = "projects.v2.floatingips_projects.create"
	}
	if method == "DELETE" {
		op.Success = []int{204}
		op.NoContentSuccess = []int{204}
	}
	return op
}
func (a *Allocations) Create(ctx context.Context, req AllocationCreateRequest, o types.CallOptions) (types.Result[[]Allocation], error) {
	return a.create(ctx, req, o, false)
}
func (a *Allocations) create(ctx context.Context, req AllocationCreateRequest, o types.CallOptions, native bool) (types.Result[[]Allocation], error) {
	raw, e := json.Marshal(map[string]any{"floatingips": req.Specs})
	count := new(big.Int)
	if native {
		count = nil
	}
	validate := func() error {
		if e != nil || req.ProjectID == "" || len(req.Specs) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		if a.scope.ProjectID != "" && a.scope.ProjectID != req.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		for _, spec := range req.Specs {
			if spec.Region == nil || *spec.Region == "" || !native && (spec.Validate() != nil || spec.Quantity == nil) {
				return types.NewError(types.InvalidArgument)
			}
			if native {
				continue
			}
			q, ok := new(big.Int).SetString(spec.Quantity.String(), 10)
			if !ok || q.Sign() <= 0 || q.Cmp(big.NewInt(2147483647)) > 0 {
				return types.NewError(types.InvalidArgument)
			}
			count.Add(count, q)
		}
		return nil
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Allocation], error) {
		out := core.Decoded[[]Allocation]{Data: []Allocation{}}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		var items []json.RawMessage
		array, hasArray := body["floatingips"]
		single, hasSingle := body["floatingip"]
		if hasArray == hasSingle {
			return out, types.NewError(types.ProtocolError)
		}
		if hasArray {
			if json.Unmarshal(array, &items) != nil || items == nil {
				return out, types.NewError(types.ProtocolError)
			}
		} else {
			items = []json.RawMessage{single}
		}
		var first error
		seen := map[string]bool{}
		for _, item := range items {
			v, e := a.decodeAllocation(item, ep, "", req.ProjectID)
			out.Refs = append(out.Refs, v.Refs...)
			if e != nil {
				if first == nil {
					first = e
				}
				continue
			}
			if seen[v.Data.Ref.ID] {
				if first == nil {
					first = types.NewError(types.ProtocolError)
				}
				continue
			}
			seen[v.Data.Ref.ID] = true
			out.Data = append(out.Data, v.Data)
		}
		if first != nil {
			return out, first
		}
		if count == nil && len(items) == 0 {
			return out, types.NewError(types.ProtocolError)
		}
		if count != nil && count.Cmp(big.NewInt(int64(len(items)))) != 0 {
			return out, types.NewError(types.Incomplete)
		}
		return out, nil
	}
	return core.DoJSON(a.runtime, ctx, a.op("create", "POST"), core.Request{Scope: a.scope, Path: "/v2/floatingips/projects/" + url.PathEscape(req.ProjectID), Body: raw, Options: o, Validate: validate}, decode)
}
func (a *Allocations) decodeAllocation(raw []byte, ep types.EndpointBinding, expected, project string) (core.Decoded[Allocation], error) {
	out := core.Decoded[Allocation]{}
	var identity struct {
		ID        *string `json:"id"`
		ProjectID *string `json:"project_id"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" || identity.ProjectID == nil || *identity.ProjectID == "" {
		return out, types.NewError(types.ProtocolError)
	}
	if expected != "" && *identity.ID != expected || project != "" && *identity.ProjectID != project || a.scope.ProjectID != "" && *identity.ProjectID != a.scope.ProjectID {
		return out, types.NewError(types.ScopeMismatch)
	}
	ref := core.Ref(ep, "projects", "floating_ip_allocation", *identity.ID)
	ref.Parent = core.Ref(ep, "projects", "project", *identity.ProjectID)
	out.Refs = []types.ResourceRef{*ref}
	var v Allocation
	if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
		return out, types.NewError(types.ProtocolError)
	}
	v.Ref = ref
	out.Data = v
	return out, nil
}

type AllocationListOptions struct{ Detailed *bool }

func (a *Allocations) validateRef(ref types.ResourceRef) error {
	if ref.Service != "projects" || ref.APIGeneration != "v2" || ref.Kind != "floating_ip_allocation" || ref.ID == "" || ref.Parent == nil || ref.Parent.ID == "" || ref.Parent.Kind != "project" || ref.Parent.Service != "projects" || ref.Parent.Parent != nil || ref.Parent.APIGeneration != "v2" || ref.Parent.Scope != ref.Scope || ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	if a.scope.ProjectID != "" && ref.Parent.ID != a.scope.ProjectID {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (a *Allocations) request(ref types.ResourceRef, o types.CallOptions) core.Request {
	return core.Request{Scope: a.scope, Path: "/v2/floatingips/" + url.PathEscape(ref.ID), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error { return a.validateRef(ref) }}
}
func (a *Allocations) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Allocation], error) {
	return core.DoJSON(a.runtime, ctx, a.op("get", "GET"), a.request(ref, o), func(raw []byte, ep types.EndpointBinding) (core.Decoded[Allocation], error) {
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil {
			return core.Decoded[Allocation]{}, types.NewError(types.ProtocolError)
		}
		return a.decodeAllocation(body["floatingip"], ep, ref.ID, ref.Parent.ID)
	})
}
func (a *Allocations) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(a.runtime, ctx, a.op("delete", "DELETE"), a.request(ref, o), func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (a *Allocations) Iterate(f AllocationListOptions, o pagination.Options) (*pagination.Iterator[Allocation], error) {
	if a.scope.ProjectID != "" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	query := url.Values{}
	if f.Detailed != nil {
		query.Set("detailed", strconv.FormatBool(*f.Detailed))
	}
	return core.FixedIterator(a.runtime, a.op("list", "GET"), core.Request{Scope: a.scope, Path: "/v2/floatingips", Query: query}, query, o, "floatingips", func(v *Allocation, ep types.EndpointBinding) error {
		raw, e := json.Marshal(v)
		if e != nil {
			return types.NewError(types.ProtocolError)
		}
		decoded, e := a.decodeAllocation(raw, ep, "", "")
		if e != nil {
			return e
		}
		*v = decoded.Data
		return nil
	})
}
func (a *Allocations) Collect(ctx context.Context, f AllocationListOptions, o pagination.Options) (pagination.Collection[Allocation], error) {
	it, e := a.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Allocation]{Items: []Allocation{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (a *Allocations) ListPage(ctx context.Context, f AllocationListOptions, o pagination.Options) (pagination.Page[Allocation], error) {
	it, e := a.Iterate(f, o)
	if e != nil {
		return pagination.Page[Allocation]{}, e
	}
	return it.Next(ctx)
}
