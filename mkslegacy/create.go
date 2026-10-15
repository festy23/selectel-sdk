package mkslegacy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"regexp"
)

func (c *Client) CreateCluster(ctx context.Context, input ClusterCreateSpec, options types.CallOptions) (types.Result[DetailedCluster], error) {
	op := core.Operation{ID: "mks.v1.clusters.create", Service: "mks", APIGeneration: "v1", Method: "POST", Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{201}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := requestBody("cluster", input)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if input.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if *input.Region != c.scope.Pool {
			return types.NewError(types.ScopeMismatch)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,31}$`).MatchString(*input.Name) {
			return types.NewError(types.InvalidArgument)
		}
		for _, group := range input.Nodegroups {
			if group.UserData != nil {
				if _, e := base64.StdEncoding.DecodeString(string(group.UserData.Reveal())); e != nil {
					return types.NewError(types.InvalidArgument)
				}
			}
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DetailedCluster], error) {
		d := core.Decoded[DetailedCluster]{}
		var env struct {
			Cluster json.RawMessage `json:"cluster"`
		}
		var id struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
			Region    *string `json:"region"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Cluster, &id) != nil || id.ID == nil || *id.ID == "" || id.ProjectID == nil || id.Region == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *id.ProjectID != c.scope.ProjectID || *id.Region != c.scope.Pool {
			return d, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "mks", "cluster", *id.ID)
		d.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(env.Cluster, &d.Data) != nil || d.Data.Validate() != nil {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Ref = ref
		return d, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
