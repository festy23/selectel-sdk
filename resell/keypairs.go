package resell

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

// KeypairsClient explicitly calls the deprecated Resell v2 SSH-key API.
// Nova keypairs are a distinct resource family with their own contract.
type KeypairsClient struct{ client *Client }
type KeypairListOptions struct{ UserID *string }

func keypairRef(endpoint types.EndpointBinding, name, user string) *types.ResourceRef {
	if name == "" || user == "" {
		return nil
	}
	ref := core.Ref(endpoint, "projects", "resell_keypair", name)
	ref.Parent = core.Ref(endpoint, "projects", "resell_user", user)
	return ref
}
func (c *KeypairsClient) Collect(ctx context.Context, filters KeypairListOptions, options pagination.Options) (pagination.Collection[Keypair], error) {
	cl := c.client
	query := url.Values{}
	if filters.UserID != nil {
		query.Set("user_id", *filters.UserID)
	}
	validate := func(v *Keypair, endpoint types.EndpointBinding) error {
		if e := v.Validate(); e != nil {
			return e
		}
		if v.Name != nil && v.UserID != nil {
			if filters.UserID != nil && *v.UserID != *filters.UserID {
				return types.NewError(types.ScopeMismatch)
			}
			v.Ref = keypairRef(endpoint, *v.Name, *v.UserID)
		}
		return nil
	}
	it, e := core.FixedIterator(cl.runtime, allocationOperation("keypairs", "list", "GET"), core.Request{Scope: cl.scope, Path: "/v2/keypairs", Query: query, Validate: cl.validateAccount}, filters, options, "keypairs", validate)
	if e != nil {
		return pagination.Collection[Keypair]{Items: []Keypair{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

// Create accepts nil for the source's optional request body. A nonnil body must
// provide the documented name, owner and public key, preserving optional regions.
func (c *KeypairsClient) Create(ctx context.Context, spec *KeypairSpec, options types.CallOptions) (types.Result[[]Keypair], error) {
	cl := c.client
	var body []byte
	if spec != nil {
		body, _ = json.Marshal(struct {
			Keypair *KeypairSpec `json:"keypair"`
		}{spec})
	}
	validate := func() error {
		if e := cl.validateAccount(); e != nil {
			return e
		}
		if spec != nil && (spec.Name == nil || *spec.Name == "" || spec.UserID == nil || *spec.UserID == "" || spec.PublicKey == nil || *spec.PublicKey == "") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[[]Keypair], err error) {
		var envelope struct {
			Keypair []json.RawMessage `json:"keypair"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Keypair == nil {
			return out, types.NewError(types.ProtocolError)
		}
		remember := func(code types.Code) {
			if err == nil {
				err = types.NewError(code)
			}
		}
		for _, row := range envelope.Keypair {
			var identity struct {
				Name   *string `json:"name"`
				UserID *string `json:"user_id"`
			}
			if json.Unmarshal(row, &identity) != nil || identity.Name == nil || identity.UserID == nil || *identity.Name == "" || *identity.UserID == "" {
				remember(types.ProtocolError)
				continue
			}
			if spec != nil && (*identity.Name != *spec.Name || *identity.UserID != *spec.UserID) {
				remember(types.ScopeMismatch)
				continue
			}
			// Repeated name/owner pairs are documented per-region variants.
			// Retain their typed results; core deduplicates Receipt identities.
			ref := keypairRef(endpoint, *identity.Name, *identity.UserID)
			out.Refs = append(out.Refs, *ref)
		}
		if err != nil {
			return out, err
		}
		out.Data = make([]Keypair, 0, len(envelope.Keypair))
		for i, row := range envelope.Keypair {
			var v Keypair
			if json.Unmarshal(row, &v) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if e := v.Validate(); e != nil {
				return out, e
			}
			v.Ref = &out.Refs[i]
			out.Data = append(out.Data, v)
		}
		return out, nil
	}
	return core.DoJSON(cl.runtime, ctx, allocationOperation("keypairs", "create", "POST"), core.Request{Scope: cl.scope, Path: "/v2/keypairs", Body: body, Options: options, Validate: validate}, decode)
}
func (c *KeypairsClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	validate := func() error {
		if e := cl.validateAccount(); e != nil {
			return e
		}
		if ref.Kind != "resell_keypair" || ref.Parent == nil || ref.Parent.Kind != "resell_user" || ref.Parent.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		p := ref.Parent
		if p.Service != ref.Service || p.APIGeneration != ref.APIGeneration || p.EndpointID != ref.EndpointID || p.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	user := ""
	if ref.Parent != nil {
		user = ref.Parent.ID
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	return core.DoJSON(cl.runtime, ctx, allocationOperation("keypairs_users", "delete", "DELETE"), core.Request{Scope: cl.scope, Path: "/v2/keypairs/" + segment(ref.ID) + "/users/" + segment(user), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}
