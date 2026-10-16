package dnslegacy

import (
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// CreatedDomain normalizes the source's direct and bind-zone response variants.
// A nil Domain is not creation proof; bound parsing errors remain protected.
type CreatedDomain struct {
	Domain    *Domain           `json:"domain,omitempty"`
	BoundZone *DomainBindResult `json:"bound_zone,omitempty"`
}

func (c *Domains) Create(ctx context.Context, spec DomainCreateSpec, o types.CallOptions) (types.Result[CreatedDomain], error) {
	spec.Name = snapshotProtected(spec.Name)
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.secrets.create", Service: "dns", APIGeneration: "v1", Method: "POST", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	body, bodyErr := wireBody(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bodyErr
	}
	bound := spec.BindZone != nil && !spec.BindZone.Empty()
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreatedDomain], error) {
		out := core.Decoded[CreatedDomain]{}
		target := domainTarget{name: spec.Name}
		if !bound {
			ref, err := domainIdentity(raw, c.scope, ep, target)
			if err != nil {
				return out, err
			}
			out.Refs = []types.ResourceRef{ref}
			var row Domain
			if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
				return out, types.NewError(types.ProtocolError)
			}
			row.Ref = &ref
			out.Data.Domain = &row
			return out, nil
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		var parent *types.ResourceRef
		var first error
		if domainRaw, ok := fields["domain"]; ok {
			known, e := domainIdentity(domainRaw, c.scope, ep, target)
			if e != nil {
				first = e
			} else {
				parent = &known
				out.Refs = append(out.Refs, known)
			}
		}
		if recordRaw, ok := fields["records"]; ok {
			var rows []json.RawMessage
			if json.Unmarshal(recordRaw, &rows) != nil || rows == nil {
				if first == nil {
					first = types.NewError(types.ProtocolError)
				}
			} else if parent != nil {
				known, e := scanRecordIDs(rows, c.scope, ep, domainTarget{ref: parent}, nil, true)
				out.Refs = append(out.Refs, known...)
				if first == nil {
					first = e
				}
			}
		}
		var row DomainBindResult
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			return out, first
		}
		if first != nil {
			return out, first
		}
		if parent != nil && row.Domain != nil {
			row.Domain.Ref = parent
			attachRecordRefs(row.Records, c.scope, ep, domainTarget{ref: parent})
		}
		out.Data = CreatedDomain{Domain: row.Domain, BoundZone: &row}
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/", Body: body, Options: o, Validate: validate}, decode)
}
