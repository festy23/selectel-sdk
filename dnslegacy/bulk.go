package dnslegacy

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func recordReference(scope types.Scope, ep types.EndpointBinding, target domainTarget, id string) *types.ResourceRef {
	if target.ref == nil {
		return nil
	}
	parent := *target.ref
	return &types.ResourceRef{Scope: scope, Service: "dns", APIGeneration: "v1", Kind: "record", ID: id, Parent: &parent, EndpointID: ep.ID, EndpointURL: ep.URL}
}
func scanRecordIDs(rows []json.RawMessage, scope types.Scope, ep types.EndpointBinding, target domainTarget, allowed map[string]bool, allowNew bool) ([]types.ResourceRef, error) {
	refs := []types.ResourceRef{}
	seen := map[string]bool{}
	var first error
	for _, raw := range rows {
		var identity struct {
			ID *types.Integer `json:"id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || !decimalID.MatchString(identity.ID.String()) {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			continue
		}
		id := identity.ID.String()
		if allowed != nil && !allowed[id] && !allowNew {
			if first == nil {
				first = types.NewError(types.ScopeMismatch)
			}
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if ref := recordReference(scope, ep, target, id); ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs, first
}
func attachRecordRefs(rows []Record, scope types.Scope, ep types.EndpointBinding, target domainTarget) {
	for i := range rows {
		if id := rows[i].RecordID(); id != nil {
			rows[i].Ref = recordReference(scope, ep, target, id.String())
		}
	}
}

// ReplaceAll explicitly replaces the desired zone contents. The source defaults
// apply_changes to false; nil and explicit false are never changed to true.
func (c *Records) ReplaceAll(ctx context.Context, parent types.ResourceRef, spec []RecordCreateSpec, applyChanges *bool, o types.CallOptions) (types.Result[ZoneSetResponse], error) {
	return c.replaceAll(ctx, domainTarget{ref: &parent}, spec, applyChanges, o)
}
func (c *Records) ReplaceAllByName(ctx context.Context, name types.ProtectedValue, spec []RecordCreateSpec, applyChanges *bool, o types.CallOptions) (types.Result[ZoneSetResponse], error) {
	return c.replaceAll(ctx, domainTarget{name: &name}, spec, applyChanges, o)
}
func (c *Records) replaceAll(ctx context.Context, target domainTarget, spec []RecordCreateSpec, applyChanges *bool, o types.CallOptions) (types.Result[ZoneSetResponse], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.replace", Service: "dns", APIGeneration: "v1", Method: "PUT", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, refs, parentErr := target.request(c.scope, op)
	body, bodyErr := wireBody(spec)
	q := url.Values{}
	if applyChanges != nil {
		q.Set("apply_changes", strconv.FormatBool(*applyChanges))
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if parentErr != nil {
			return parentErr
		}
		if spec == nil {
			return types.NewError(types.InvalidArgument)
		}
		for _, row := range spec {
			if row.Validate() != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return bodyErr
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[ZoneSetResponse], error) {
		out := core.Decoded[ZoneSetResponse]{}
		var envelope struct {
			Applied *bool                      `json:"applied"`
			Changes map[string]json.RawMessage `json:"changes"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Applied == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Applied = envelope.Applied
		var first error
		if *envelope.Applied {
			for _, field := range []string{"create", "update", "delete"} {
				var rows []json.RawMessage
				if json.Unmarshal(envelope.Changes[field], &rows) != nil || rows == nil {
					if first == nil {
						first = types.NewError(types.ProtocolError)
					}
					continue
				}
				known, e := scanRecordIDs(rows, c.scope, ep, target, nil, true)
				out.Refs = append(out.Refs, known...)
				if first == nil {
					first = e
				}
			}
		}
		var row ZoneSetResponse
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			return out, first
		}
		if first != nil {
			return out, first
		}
		if *row.Applied {
			attachRecordRefs(row.Changes.Create, c.scope, ep, target)
			attachRecordRefs(row.Changes.Update, c.scope, ep, target)
			attachRecordRefs(row.Changes.Delete, c.scope, ep, target)
		}
		out.Data = row
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/records", Query: q, Refs: refs, Body: body, Options: o, Validate: validate}, decode)
}

func (c *Records) Batch(ctx context.Context, parent types.ResourceRef, spec RecordBatchSpec, o types.CallOptions) (types.Result[[]Record], error) {
	return c.batch(ctx, domainTarget{ref: &parent}, spec, o)
}
func (c *Records) BatchByName(ctx context.Context, name types.ProtectedValue, spec RecordBatchSpec, o types.CallOptions) (types.Result[[]Record], error) {
	return c.batch(ctx, domainTarget{name: &name}, spec, o)
}
func (c *Records) batch(ctx context.Context, target domainTarget, spec RecordBatchSpec, o types.CallOptions) (types.Result[[]Record], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records_batch_update.update", Service: "dns", APIGeneration: "v1", Method: "PATCH", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, refs, targetErr := target.request(c.scope, op)
	body, bodyErr := wireBody(spec)
	allowed := map[string]bool{}
	var idErr error
	addID := func(id *types.Integer) {
		if id == nil || !decimalID.MatchString(id.String()) {
			idErr = types.NewError(types.InvalidArgument)
			return
		}
		allowed[id.String()] = true
		if ref := recordReference(c.scope, types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint}, target, id.String()); ref != nil {
			refs = append(refs, *ref)
		}
	}
	for _, row := range spec.Update {
		addID(row.RecordID())
	}
	for _, row := range spec.Delete {
		addID(row.ID)
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if targetErr != nil {
			return targetErr
		}
		if spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if idErr != nil {
			return idErr
		}
		return bodyErr
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Record], error) {
		out := core.Decoded[[]Record]{}
		var identities []json.RawMessage
		if json.Unmarshal(raw, &identities) != nil || identities == nil {
			return out, types.NewError(types.ProtocolError)
		}
		refs, first := scanRecordIDs(identities, c.scope, ep, target, allowed, len(spec.Create) > 0)
		out.Refs = refs
		var rows []Record
		if json.Unmarshal(raw, &rows) != nil {
			if first == nil {
				first = types.NewError(types.ProtocolError)
			}
			return out, first
		}
		if first != nil {
			return out, first
		}
		attachRecordRefs(rows, c.scope, ep, target)
		out.Data = rows
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/records/batch_update", Refs: refs, Body: body, Options: o, Validate: validate}, decode)
}
