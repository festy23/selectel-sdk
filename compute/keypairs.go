package compute

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Keypairs struct {
	runtime *core.Runtime
	scope   types.Scope
	Current *CurrentKeypairs
}
type CurrentKeypairs struct{ keys *Keypairs }
type KeypairFilters struct{ UserID, Microversion string }
type KeypairCreateRequest struct {
	Name, UserID, Type, Microversion string
	PublicKey                        *string
}

func bindKeypairs(r *core.Runtime, s types.Scope) *Keypairs {
	k := &Keypairs{runtime: r, scope: s}
	k.Current = &CurrentKeypairs{keys: k}
	return k
}
func keyOperation(action, method, version string) (core.Operation, error) {
	if version == "" {
		version = "2.35"
	}
	_, v, e := microversion(version)
	op := operation(action, method)
	op.ID = "compute.v2_1.keypairs." + action
	op.Headers.Set("OpenStack-API-Version", "compute "+v)
	op.Headers.Set("X-OpenStack-Nova-API-Version", v)
	if method == "POST" {
		op.Success = []int{200, 201}
		op.Rejected = []int{400, 401, 403, 409}
	}
	op.NoContentSuccess = nil
	if method == "DELETE" {
		op.NoContentSuccess = []int{204}
	}
	return op, e
}
func keyRef(endpoint types.EndpointBinding, name, owner string, current bool) *types.ResourceRef {
	reference := core.Ref(endpoint, "compute", "keypair", name)
	if reference == nil {
		return nil
	}
	kind := "keypair_user"
	if current {
		kind = "keypair_current_user"
		owner = "current"
	}
	reference.Parent = core.Ref(endpoint, "compute", kind, owner)
	return reference
}
func keyOwner(reference types.ResourceRef, current bool) (string, error) {
	if reference.Kind != "keypair" || reference.Parent == nil || reference.Parent.Parent != nil || reference.Parent.Scope != reference.Scope || reference.Parent.Service != "compute" || reference.Parent.APIGeneration != "v2_1" || reference.Parent.EndpointID != reference.EndpointID || reference.Parent.EndpointURL != reference.EndpointURL {
		return "", types.NewError(types.InvalidArgument)
	}
	if current {
		if reference.Parent.Kind != "keypair_current_user" || reference.Parent.ID != "current" {
			return "", types.NewError(types.InvalidArgument)
		}
		return "", nil
	}
	if reference.Parent.Kind != "keypair_user" || reference.Parent.ID == "" {
		return "", types.NewError(types.InvalidArgument)
	}
	return reference.Parent.ID, nil
}
func decodeKey(raw []byte, endpoint types.EndpointBinding, name, owner string, current bool) (Keypair, error) {
	var body struct {
		Keypair *Keypair `json:"keypair"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Keypair == nil || body.Keypair.Validate() != nil || body.Keypair.Name == nil || *body.Keypair.Name == "" {
		return Keypair{}, types.NewError(types.ProtocolError)
	}
	v := body.Keypair
	if name != "" && *v.Name != name {
		return Keypair{}, types.NewError(types.ScopeMismatch)
	}
	if v.UserID != nil && *v.UserID != "" {
		if owner != "" && *v.UserID != owner {
			return Keypair{}, types.NewError(types.ScopeMismatch)
		}
		owner = *v.UserID
		current = false
	}
	v.Ref = keyRef(endpoint, *v.Name, owner, current)
	if v.Ref == nil || v.Ref.Parent == nil {
		return Keypair{}, types.NewError(types.ProtocolError)
	}
	return *v, nil
}
func (k *Keypairs) create(ctx context.Context, r KeypairCreateRequest, o types.CallOptions, current bool) (types.Result[Keypair], error) {
	op, versionErr := keyOperation("create", "POST", r.Microversion)
	minor, _, _ := microversion(strings.TrimPrefix(op.Headers.Get("OpenStack-API-Version"), "compute "))
	body := map[string]any{"name": r.Name}
	if r.UserID != "" {
		body["user_id"] = r.UserID
	}
	if r.Type != "" {
		body["type"] = r.Type
	}
	if r.PublicKey != nil {
		body["public_key"] = *r.PublicKey
	}
	raw, _ := json.Marshal(map[string]any{"keypair": body})
	validate := func() error {
		if versionErr != nil {
			return versionErr
		}
		if r.Name == "" || strings.ContainsAny(r.Name, "/\x00\r\n") || current != (r.UserID == "") || r.UserID != "" && minor < 10 || r.Type != "" && minor < 2 || r.Type != "" && r.Type != "ssh" && r.Type != "x509" || minor >= 92 && r.PublicKey == nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	result, e := core.DoJSON(k.runtime, ctx, op, core.Request{Scope: k.scope, Path: "/os-keypairs", Body: raw, Options: o, Validate: validate}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Keypair], error) {
		// Keep the exact accepted identity even if an unrelated sensitive field is malformed.
		var envelope map[string]json.RawMessage
		var id struct {
			Name   string `json:"name"`
			UserID string `json:"user_id"`
		}
		partial := core.Decoded[Keypair]{}
		if json.Unmarshal(raw, &envelope) == nil && json.Unmarshal(envelope["keypair"], &id) == nil && id.Name == r.Name && (id.UserID == "" || r.UserID == "" || id.UserID == r.UserID) {
			owner, relative := r.UserID, current
			if id.UserID != "" {
				owner = id.UserID
				relative = false
			}
			if reference := keyRef(endpoint, r.Name, owner, relative); reference != nil && reference.Parent != nil {
				partial.Refs = []types.ResourceRef{*reference}
			}
		}
		value, err := decodeKey(raw, endpoint, r.Name, r.UserID, current)
		if err != nil {
			return partial, err
		}
		partial.Data = value
		partial.Refs = []types.ResourceRef{*value.Ref}
		return partial, nil
	})
	if result.Receipt != nil && result.Receipt.Endpoint != nil && len(result.Receipt.ResourceRefs) == 0 {
		reference := keyRef(*result.Receipt.Endpoint, r.Name, r.UserID, current)
		if reference != nil && reference.Parent != nil {
			result.Receipt.ResourceRefs = []types.ResourceRef{*reference}
			result.Receipt.Recovery = "resource"
		}
	}
	return result, e
}
func (k *Keypairs) Create(ctx context.Context, r KeypairCreateRequest, o types.CallOptions) (types.Result[Keypair], error) {
	return k.create(ctx, r, o, false)
}
func (k *CurrentKeypairs) Create(ctx context.Context, r KeypairCreateRequest, o types.CallOptions) (types.Result[Keypair], error) {
	return k.keys.create(ctx, r, o, true)
}
func (k *Keypairs) get(ctx context.Context, reference types.ResourceRef, o types.CallOptions, current bool) (types.Result[Keypair], error) {
	owner, err := keyOwner(reference, current)
	query := url.Values{}
	if owner != "" {
		query.Set("user_id", owner)
	}
	op, _ := keyOperation("get", "GET", "")
	return core.DoJSON(k.runtime, ctx, op, core.Request{Scope: k.scope, Path: "/os-keypairs/" + url.PathEscape(reference.ID), Query: query, Options: o, Refs: []types.ResourceRef{reference}, PinnedEndpoint: pin(reference), Validate: func() error { return err }}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Keypair], error) {
		v, e := decodeKey(raw, endpoint, reference.ID, owner, current)
		return core.Decoded[Keypair]{Data: v}, e
	})
}
func (k *Keypairs) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Keypair], error) {
	return k.get(ctx, r, o, false)
}
func (k *CurrentKeypairs) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Keypair], error) {
	return k.keys.get(ctx, r, o, true)
}
func (k *Keypairs) remove(ctx context.Context, r types.ResourceRef, o types.CallOptions, current bool) (types.Result[struct{}], error) {
	owner, err := keyOwner(r, current)
	query := url.Values{}
	if owner != "" {
		query.Set("user_id", owner)
	}
	op, _ := keyOperation("delete", "DELETE", "")
	return core.DoJSON(k.runtime, ctx, op, core.Request{Scope: k.scope, Path: "/os-keypairs/" + url.PathEscape(r.ID), Query: query, Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return err }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(strings.TrimSpace(string(raw))) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	})
}
func (k *Keypairs) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return k.remove(ctx, r, o, false)
}
func (k *CurrentKeypairs) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return k.keys.remove(ctx, r, o, true)
}
func (k *Keypairs) iterate(filters KeypairFilters, o pagination.Options, current bool) (*pagination.Iterator[Keypair], error) {
	op, err := keyOperation("list", "GET", filters.Microversion)
	if err != nil {
		return nil, err
	}
	minor, _, _ := microversion(strings.TrimPrefix(op.Headers.Get("OpenStack-API-Version"), "compute "))
	if current != (filters.UserID == "") || filters.UserID != "" && minor < 10 {
		return nil, types.NewError(types.InvalidArgument)
	}
	plan, err := k.runtime.PagePlan(op.ID, "v2_1", k.scope, "", struct {
		Filters KeypairFilters
		Current bool
	}{filters, current})
	if err != nil {
		return nil, err
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	plan.FixedSize = minor < 35
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[Keypair], error) {
		query := url.Values{}
		if filters.UserID != "" {
			query.Set("user_id", filters.UserID)
		}
		if minor >= 35 {
			query.Set("limit", strconv.FormatUint(uint64(size), 10))
			if marker != "" {
				query.Set("marker", marker)
			}
		}
		next := ""
		done := true
		result, e := core.DoJSON(k.runtime, ctx, op, core.Request{Scope: k.scope, Path: "/os-keypairs", Query: query, ExpectedEndpointID: expected}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]Keypair], error) {
			var body struct {
				Pairs []json.RawMessage `json:"keypairs"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Pairs == nil {
				return core.Decoded[[]Keypair]{}, types.NewError(types.ProtocolError)
			}
			items := make([]Keypair, 0, len(body.Pairs))
			for _, item := range body.Pairs {
				v, e := decodeKey(item, endpoint, "", filters.UserID, current)
				if e != nil {
					return core.Decoded[[]Keypair]{}, e
				}
				items = append(items, v)
			}
			if minor >= 35 && len(items) > 0 {
				next = *items[len(items)-1].Name
				done = false
			}
			return core.Decoded[[]Keypair]{Data: items}, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[Keypair]{Items: result.Data, Next: next, Done: done, Metadata: result.Metadata}, e
	}
	return pagination.New(plan, o, fetch)
}
func (k *Keypairs) Iterate(f KeypairFilters, o pagination.Options) (*pagination.Iterator[Keypair], error) {
	return k.iterate(f, o, false)
}
func (k *CurrentKeypairs) Iterate(o pagination.Options) (*pagination.Iterator[Keypair], error) {
	return k.keys.iterate(KeypairFilters{}, o, true)
}
func (k *Keypairs) Collect(ctx context.Context, f KeypairFilters, o pagination.Options) (pagination.Collection[Keypair], error) {
	it, e := k.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Keypair]{Items: []Keypair{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (k *CurrentKeypairs) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[Keypair], error) {
	it, e := k.Iterate(o)
	if e != nil {
		return pagination.Collection[Keypair]{Items: []Keypair{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (k *Keypairs) ListPage(ctx context.Context, f KeypairFilters, o pagination.Options) (pagination.Page[Keypair], error) {
	it, e := k.Iterate(f, o)
	if e != nil {
		return pagination.Page[Keypair]{}, e
	}
	return it.Next(ctx)
}
func (k *CurrentKeypairs) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[Keypair], error) {
	it, e := k.Iterate(o)
	if e != nil {
		return pagination.Page[Keypair]{}, e
	}
	return it.Next(ctx)
}
