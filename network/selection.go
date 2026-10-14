package network

import (
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

type GetOptions[F ~string] struct{ Fields []F }
type NetworkGetOptions = GetOptions[NetworkField]
type SubnetGetOptions = GetOptions[SubnetField]
type PortGetOptions = GetOptions[PortField]
type NetworkUpdateRequest struct {
	Ref      types.ResourceRef
	Spec     NetworkUpdateSpec
	Revision *types.Integer
}
type DeleteRequest struct {
	Ref      types.ResourceRef
	Revision *types.Integer
}

func fieldQuery[F ~string](fields []F, valid func(F) bool) (url.Values, error) {
	q := url.Values{}
	for _, f := range fields {
		if !valid(f) {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("fields", string(f))
	}
	return q, nil
}
func validateRef(r types.ResourceRef, kind string) error {
	if r.Kind != kind || r.ID == "" || r.Service != "network" || r.APIGeneration != "v2" {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "network" {
		if r.Parent != nil {
			return types.NewError(types.ScopeMismatch)
		}
	} else {
		if r.Parent == nil || validateRef(*r.Parent, "network") != nil || r.Parent.Scope != r.Scope || r.Parent.EndpointID != r.EndpointID || r.Parent.EndpointURL != r.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func requestForRef(scope types.Scope, ref types.ResourceRef, path string, o types.CallOptions) core.Request {
	return core.Request{Scope: scope, Path: path, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, ExpectedEndpointID: ref.EndpointID}
}
func revisionHeader(op *core.Operation, revision *types.Integer) error {
	if revision == nil {
		return nil
	}
	if revision.String() == "" || revision.String()[0] == '-' {
		return types.NewError(types.InvalidArgument)
	}
	op.Headers = http.Header{"If-Match": []string{"revision_number=" + revision.String()}}
	return nil
}
