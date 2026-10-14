package network

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type model interface{ Validate() error }
type decodeOptions[T model] struct {
	Kind       string
	Expected   *types.ResourceRef
	Parent     *types.ResourceRef
	Owner      string
	Projection bool
	SetRef     func(*T, *types.ResourceRef)
}

func decodeItem[T model](raw []byte, endpoint types.EndpointBinding, o decodeOptions[T]) (core.Decoded[T], error) {
	var identity struct {
		ID        *string `json:"id"`
		ProjectID *string `json:"project_id"`
		TenantID  *string `json:"tenant_id"`
		NetworkID *string `json:"network_id"`
	}
	out := core.Decoded[T]{}
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &identity) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	if e := responseOwner(identity.ProjectID, identity.TenantID, o.Owner); e != nil {
		return out, e
	}
	id := ""
	if identity.ID != nil {
		if *identity.ID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		id = *identity.ID
	}
	if o.Expected != nil {
		if id != "" && id != o.Expected.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		if id == "" && o.Projection {
			id = o.Expected.ID
		}
	}
	missingIdentity := id == "" && !o.Projection
	parent := o.Parent
	if parent == nil && o.Expected != nil {
		parent = o.Expected.Parent
	}
	parentMissing := false
	if o.Kind != "network" {
		if identity.NetworkID != nil {
			if *identity.NetworkID == "" {
				return out, types.NewError(types.ProtocolError)
			}
			if parent != nil && *identity.NetworkID != parent.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			parent = core.Ref(endpoint, "network", "network", *identity.NetworkID)
		} else if !o.Projection {
			parentMissing = true
		}
	}
	var reference *types.ResourceRef
	if id != "" && (o.Kind == "network" || parent != nil) {
		reference = core.Ref(endpoint, "network", o.Kind, id)
		if parent != nil {
			copy := *parent
			reference.Parent = &copy
		}
		out.Refs = []types.ResourceRef{*reference}
	}
	if missingIdentity || parentMissing {
		return out, types.NewError(types.ProtocolError)
	}
	var value T
	if json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return out, types.NewError(types.ProtocolError)
	}
	o.SetRef(&value, reference)
	out.Data = value
	return out, nil
}
func decodeEnvelope[T model](kind string, o decodeOptions[T]) core.Decoder[T] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[T], error) {
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		return decodeItem(envelope[kind], endpoint, o)
	}
}
