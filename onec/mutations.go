package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
)

func (c *Client) CreateCluster(ctx context.Context, spec CreateClusterRequest, options types.CallOptions) (types.Result[CreateClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters", Body: body, Options: options, Validate: func() error {
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[CreateClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil || *identity.Cluster.ID == "" {
			return core.Decoded[CreateClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[CreateClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(endpoint, "cloud_1c", "cluster", *identity.Cluster.ID)
		out := core.Decoded[CreateClusterResponse]{Refs: []types.ResourceRef{*ref}}
		var v CreateClusterResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Cluster.Ref = ref
		out.Data = v
		return out, nil
	})
}
