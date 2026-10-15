package filestorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"reflect"
)

func (c *Client) CreateNetwork(ctx context.Context, network, subnet types.ResourceRef, input NetworkCreateRequest, options types.CallOptions) (types.Result[ShareNetwork], error) {
	op := core.Operation{ID: "filestorage.v2_73.share_networks.create", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200, 202}, Rejected: []int{400, 401, 403, 413, 422}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	originalNet, originalSubnet := input.NeutronNetID, input.NeutronSubnetID
	input.NeutronNetID = &network.ID
	input.NeutronSubnetID = &subnet.ID
	body, bodyErr := wireBody(struct {
		Network NetworkCreateRequest `json:"share_network"`
	}{input})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/share-networks", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateNetworkOrigin(network, subnet); e != nil {
			return e
		}
		if originalNet != nil && *originalNet != network.ID || originalSubnet != nil && *originalSubnet != subnet.ID {
			return types.NewError(types.ScopeMismatch)
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ShareNetwork], error) {
		decoded := core.Decoded[ShareNetwork]{}
		var envelope struct {
			Network json.RawMessage `json:"share_network"`
		}
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if json.Unmarshal(raw, &envelope) != nil || json.Unmarshal(envelope.Network, &identity) != nil || identity.ID == nil || *identity.ID == "" || identity.ProjectID == nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.ProjectID != c.scope.ProjectID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "filestorage", "share_network", *identity.ID)
		decoded.Refs = []types.ResourceRef{*ref}
		// The pinned create contract explicitly creates a default share-network
		// subnet. Preserve every independently trustworthy returned child ID.
		var children struct {
			Subnets json.RawMessage `json:"share_network_subnets"`
		}
		if json.Unmarshal(envelope.Network, &children) != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		if len(children.Subnets) > 0 {
			var rows []json.RawMessage
			if json.Unmarshal(children.Subnets, &rows) != nil || rows == nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			var firstErr error
			for _, row := range rows {
				var childID struct {
					ID *string `json:"id"`
				}
				if json.Unmarshal(row, &childID) != nil || childID.ID == nil || *childID.ID == "" || seen[*childID.ID] {
					if firstErr == nil {
						firstErr = types.NewError(types.ProtocolError)
					}
					continue
				}
				seen[*childID.ID] = true
				child := core.Ref(ep, "filestorage", "share_network_subnet", *childID.ID)
				parent := *ref
				child.Parent = &parent
				decoded.Refs = append(decoded.Refs, *child)
			}
			if firstErr != nil {
				return decoded, firstErr
			}
		}
		if json.Unmarshal(envelope.Network, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		for _, child := range decoded.Data.ShareNetworkSubnets {
			if child.NeutronNetID != nil {
				if id, valid := child.NeutronNetID.Value(); valid && id != network.ID {
					return decoded, types.NewError(types.ScopeMismatch)
				}
			}
			if child.NeutronSubnetID != nil {
				if id, valid := child.NeutronSubnetID.Value(); valid && id != subnet.ID {
					return decoded, types.NewError(types.ScopeMismatch)
				}
			}
		}
		decoded.Data.Ref = ref
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
		if c.validateNetworkOrigin(network, subnet) == nil {
			out.Receipt.ResourceRefs = append(out.Receipt.ResourceRefs, cloneRef(network), cloneRef(subnet))
		}
	}
	return out, e
}

func (c *Client) validateNetworkOrigin(network, subnet types.ResourceRef) error {
	for _, ref := range []types.ResourceRef{network, subnet} {
		if ref.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if ref.Scope != c.scope || ref.Service != "network" || ref.APIGeneration != "v2" || ref.Scope.ProjectID == "" || ref.Scope.Region == "" || ref.Scope.Pool == "" {
			return types.NewError(types.ScopeMismatch)
		}
		for ancestor := ref.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Service != ref.Service || ancestor.APIGeneration != ref.APIGeneration || ancestor.EndpointID != ref.EndpointID || ancestor.EndpointURL != ref.EndpointURL {
				return types.NewError(types.ScopeMismatch)
			}
		}
	}
	if network.Kind != "network" || network.Parent != nil || subnet.Kind != "subnet" || network.EndpointID != subnet.EndpointID || network.EndpointURL != subnet.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	if subnet.Parent != nil && !reflect.DeepEqual(*subnet.Parent, network) {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func cloneRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		parent := cloneRef(*ref.Parent)
		ref.Parent = &parent
	}
	return ref
}
