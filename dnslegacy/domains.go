package dnslegacy

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Domains struct {
	runtime *core.Runtime
	scope   types.Scope
}

// DomainSelector selects either an exact numeric provider ID or an explicitly
// requested domain name. Names are never used to recover an unknown mutation.
type DomainSelector struct {
	ID   string
	Name *types.ProtectedValue
}

func (s DomainSelector) path() (string, error) {
	if s.ID != "" {
		if s.Name != nil || !decimalID.MatchString(s.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "/" + s.ID, nil
	}
	if s.Name == nil || s.Name.Empty() || !utf8.Valid(s.Name.Reveal()) {
		return "", types.NewError(types.InvalidArgument)
	}
	return "/" + url.PathEscape(string(s.Name.Reveal())), nil
}

func (c *Domains) Get(ctx context.Context, selector DomainSelector, showIPs *bool, o types.CallOptions) (types.Result[Domain], error) {
	return c.get(ctx, selector, showIPs, nil, o)
}
func (c *Domains) GetRef(ctx context.Context, ref types.ResourceRef, showIPs *bool, o types.CallOptions) (types.Result[Domain], error) {
	return c.get(ctx, DomainSelector{ID: ref.ID}, showIPs, &ref, o)
}
func (c *Domains) get(ctx context.Context, selector DomainSelector, showIPs *bool, expected *types.ResourceRef, o types.CallOptions) (types.Result[Domain], error) {
	selector.Name = snapshotProtected(selector.Name)
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.secrets.get", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	path, selectorErr := selector.path()
	q := url.Values{}
	if showIPs != nil {
		if *showIPs {
			q.Set("show_ips", "true")
		} else {
			q.Set("show_ips", "false")
		}
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[Domain], error) {
		var row Domain
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil || !decimalID.MatchString(row.ID.String()) {
			return core.Decoded[Domain]{}, types.NewError(types.ProtocolError)
		}
		if selector.ID != "" && selector.ID != row.ID.String() {
			return core.Decoded[Domain]{}, types.NewError(types.ScopeMismatch)
		}
		if selector.Name != nil && !strings.EqualFold(strings.TrimSuffix(string(selector.Name.Reveal()), "."), strings.TrimSuffix(string(row.Name.Reveal()), ".")) {
			return core.Decoded[Domain]{}, types.NewError(types.ScopeMismatch)
		}
		ref := types.ResourceRef{Scope: c.scope, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: row.ID.String(), EndpointID: ep.ID, EndpointURL: ep.URL}
		row.Ref = &ref
		return core.Decoded[Domain]{Data: row, Refs: []types.ResourceRef{ref}}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Query: q, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if expected != nil {
			_, _, e := (domainTarget{ref: expected}).request(c.scope, op)
			if e != nil {
				return e
			}
		}
		return selectorErr
	}}, decode)
}
