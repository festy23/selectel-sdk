package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) ListClusterSessions(ctx context.Context, cluster types.ResourceRef, infobaseID *string, options pagination.Options) (pagination.Collection[ClusterSession], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_sessions.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[ClusterSession]{}, e
	}
	if e := c.validateClusterAnchor(cluster, op); e != nil {
		return pagination.Collection[ClusterSession]{}, e
	}
	query := url.Values{}
	filter := ""
	if infobaseID != nil {
		filter = *infobaseID
		query.Set("infobaseId", filter)
	}
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/sessions", Query: query, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint}, struct {
		Cluster types.ResourceRef
		Filter  string
		Present bool
	}{cluster, filter, infobaseID != nil}, options, "sessions", func(v *ClusterSession, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.SessionID == nil || *v.SessionID == "" {
			return types.NewError(types.ProtocolError)
		}
		if filter != "" && v.InfobaseID != nil && *v.InfobaseID != filter {
			return types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(ep, "cloud_1c", "cluster_session", *v.SessionID)
		v.Ref.Parent = cloneResourceRef(&cluster)
		return nil
	})
	if e != nil {
		return pagination.Collection[ClusterSession]{}, e
	}
	return iterator.Collect(ctx)
}

// TerminateClusterSessions requires exactly one explicit ID or filter. A
// deliberately supplied empty filter selects all sessions, as the source defines.
// Acceptance does not establish termination: no source completion read is claimed.
func (c *Client) TerminateClusterSessions(ctx context.Context, cluster types.ResourceRef, spec TerminateClusterSessionBody, options types.CallOptions) (types.Result[TerminateClusterSessionResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_sessions_terminate.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(spec)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	refs := []types.ResourceRef{cluster}
	if spec.SessionID != nil && *spec.SessionID != "" {
		session := cluster
		session.Kind = "cluster_session"
		session.ID = *spec.SessionID
		if cluster.Validate() == nil {
			session.Parent = cloneResourceRef(&cluster)
		}
		refs = append(refs, session)
	}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/sessions/terminate", Body: body, Options: options, Refs: refs, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterAnchor(cluster, op); e != nil {
			return e
		}
		if bodyErr != nil || spec.Validate() != nil || (spec.SessionID == nil) == (spec.Filter == nil) || spec.SessionID != nil && *spec.SessionID == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[TerminateClusterSessionResponse], error) {
		var v TerminateClusterSessionResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[TerminateClusterSessionResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[TerminateClusterSessionResponse]{Data: v}, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
