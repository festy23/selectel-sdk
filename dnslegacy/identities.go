package dnslegacy

import (
	"encoding/json"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func sameDomainName(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}
func domainIdentity(raw []byte, scope types.Scope, ep types.EndpointBinding, target domainTarget) (types.ResourceRef, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return types.ResourceRef{}, types.NewError(types.ProtocolError)
	}
	var id types.Integer
	if json.Unmarshal(fields["id"], &id) != nil || !decimalID.MatchString(id.String()) {
		return types.ResourceRef{}, types.NewError(types.ProtocolError)
	}
	if target.ref != nil && target.ref.ID != id.String() {
		return types.ResourceRef{}, types.NewError(types.ScopeMismatch)
	}
	if target.name != nil {
		var name string
		if json.Unmarshal(fields["name"], &name) != nil {
			return types.ResourceRef{}, types.NewError(types.ProtocolError)
		}
		if !sameDomainName(name, string(target.name.Reveal())) {
			return types.ResourceRef{}, types.NewError(types.ScopeMismatch)
		}
	}
	return types.ResourceRef{Scope: scope, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: id.String(), EndpointID: ep.ID, EndpointURL: ep.URL}, nil
}
func decodeDomain(scope types.Scope, target domainTarget) core.Decoder[Domain] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Domain], error) {
		ref, err := domainIdentity(raw, scope, ep, target)
		if err != nil {
			return core.Decoded[Domain]{}, err
		}
		out := core.Decoded[Domain]{Refs: []types.ResourceRef{ref}}
		var row Domain
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		row.Ref = &ref
		out.Data = row
		return out, nil
	}
}
