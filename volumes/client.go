// Package volumes binds the confirmed Selectel blockstorage SDK to Cinder v3.
package volumes

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/http"
	"net/url"
	"unicode/utf8"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
	Types   *Types
	listPin *types.EndpointBinding
}
type Types struct{ client *Client }

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Types = &Types{c}
	return c
}

type ExtendSpec struct {
	NewSize  types.Integer
	Attached bool
}

func operation(action, method string) core.Operation {
	effect := "read"
	success := 200
	if method != "GET" {
		effect = "mutation"
		success = 202
		if method == "PUT" {
			success = 200
		}
	}
	return core.Operation{ID: "volumes.v3." + action, Service: "volumes", APIGeneration: "v3", Method: method, Effect: effect, Auth: types.IAMProject, ServiceType: "volumev3", RequireProject: true, RequireLocation: true, Success: []int{success}, NoContentSuccess: func() []int {
		if action == "volumes.extend" || action == "volumes.delete" {
			return []int{202}
		}
		return nil
	}(), Rejected: []int{400, 401, 403, 404, 405, 406, 409, 413, 422, 429}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 405: types.InvalidArgument, 406: types.CapabilityUnavailable, 413: types.InvalidArgument, 422: types.InvalidArgument}, Headers: http.Header{"Openstack-Api-Version": {"volume 3.0"}}, ErrorProtocol: "cinder"}
}
func invalid() error { return types.NewError(types.InvalidArgument) }
func validateRef(r types.ResourceRef, kind string) error {
	if r.Validate() != nil || r.Service != "volumes" || r.APIGeneration != "v3" || r.Kind != kind || r.Parent != nil {
		return invalid()
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func positive(v types.Integer) bool {
	n, ok := new(big.Int).SetString(v.String(), 10)
	return ok && n.Sign() > 0
}
func metadata(m map[string]string) error {
	for k, v := range m {
		if utf8.RuneCountInString(k) > 255 || utf8.RuneCountInString(v) > 255 {
			return invalid()
		}
	}
	return nil
}
func (c *Client) Create(ctx context.Context, s CreateSpec, o types.CallOptions) (types.Result[Volume], error) {
	e := metadata(s.Metadata)
	count := 0
	for _, id := range []string{s.SnapshotID, s.SourceVolID, s.ImageID, s.BackupID} {
		if id != "" {
			count++
		}
	}
	if count > 1 || (s.Size != nil && !positive(*s.Size)) {
		e = invalid()
	}
	if s.Size == nil && s.SnapshotID == "" && s.SourceVolID == "" && s.BackupID == "" {
		e = invalid()
	}
	raw, _ := json.Marshal(struct {
		Volume CreateSpec `json:"volume"`
	}{s})
	op := operation("volumes.create", "POST")
	if s.BackupID != "" {
		op.Headers.Set("OpenStack-API-Version", "volume 3.47")
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/volumes", Body: raw, Options: o, Validate: func() error { return e }}, decodeVolume(""))
}
func decodeVolume(expected string) core.Decoder[Volume] {
	return func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Volume], error) {
		var body struct {
			Volume json.RawMessage `json:"volume"`
		}
		out := core.Decoded[Volume]{}
		if json.Unmarshal(raw, &body) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body.Volume, &identity) == nil && identity.ID != "" {
			if expected != "" && identity.ID != expected {
				return out, types.NewError(types.ScopeMismatch)
			}
			out.Refs = []types.ResourceRef{*core.Ref(endpoint, "volumes", "volume", identity.ID)}
		}
		var v Volume
		if json.Unmarshal(body.Volume, &v) != nil || v.ID == "" || v.Size.String() == "" {
			return out, types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(endpoint, "volumes", "volume", v.ID)
		out.Data = v
		return out, nil
	}
}
func (c *Client) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Volume], error) {
	return core.DoJSON(c.runtime, ctx, operation("volumes.get", "GET"), core.Request{Scope: c.scope, Path: "/volumes/" + url.PathEscape(r.ID), Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return validateRef(r, "volume") }}, decodeVolume(r.ID))
}
func (c *Client) Update(ctx context.Context, r types.ResourceRef, s UpdateSpec, o types.CallOptions) (types.Result[Volume], error) {
	e := validateRef(r, "volume")
	if s.Name == nil && s.Description == nil && s.Metadata == nil {
		e = invalid()
	}
	if s.Metadata != nil {
		if *s.Metadata == nil {
			e = invalid()
		}
		if err := metadata(*s.Metadata); err != nil {
			e = err
		}
	}
	raw, _ := json.Marshal(struct {
		Volume UpdateSpec `json:"volume"`
	}{s})
	return core.DoJSON(c.runtime, ctx, operation("volumes.update", "PUT"), core.Request{Scope: c.scope, Path: "/volumes/" + url.PathEscape(r.ID), Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Body: raw, Validate: func() error { return e }}, decodeVolume(r.ID))
}
func (c *Client) Extend(ctx context.Context, r types.ResourceRef, s ExtendSpec, o types.CallOptions) (types.Result[Empty], error) {
	e := validateRef(r, "volume")
	if !positive(s.NewSize) {
		e = invalid()
	}
	body, _ := json.Marshal(struct {
		Extend struct {
			NewSize types.Integer `json:"new_size"`
		} `json:"os-extend"`
	}{Extend: struct {
		NewSize types.Integer `json:"new_size"`
	}{s.NewSize}})
	op := operation("volumes.extend", "POST")
	if s.Attached {
		op.Headers.Set("OpenStack-API-Version", "volume 3.42")
	}
	return core.DoJSON[Empty](c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/volumes/" + url.PathEscape(r.ID) + "/action", Body: body, Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return e }}, emptyDecoder)
}
func (c *Client) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return core.DoJSON[Empty](c.runtime, ctx, operation("volumes.delete", "DELETE"), core.Request{Scope: c.scope, Path: "/volumes/" + url.PathEscape(r.ID), Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return validateRef(r, "volume") }}, emptyDecoder)
}
func decodeType(raw []byte, ep types.EndpointBinding, expected string) (VolumeType, error) {
	var v VolumeType
	if json.Unmarshal(raw, &v) != nil || v.ID == "" {
		return VolumeType{}, types.NewError(types.ProtocolError)
	}
	if expected != "" && expected != "default" && expected != v.ID {
		return VolumeType{}, types.NewError(types.ScopeMismatch)
	}
	v.Ref = core.Ref(ep, "volumes", "volume_type", v.ID)
	return v, nil
}

// Default reads the configured default type and returns its concrete provider ID.
func (t *Types) Default(ctx context.Context, o types.CallOptions) (types.Result[VolumeType], error) {
	return t.get(ctx, "default", nil, o)
}
func (t *Types) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[VolumeType], error) {
	return t.get(ctx, r.ID, &r, o)
}
func (t *Types) get(ctx context.Context, id string, r *types.ResourceRef, o types.CallOptions) (types.Result[VolumeType], error) {
	in := core.Request{Scope: t.client.scope, Path: "/types/" + url.PathEscape(id), Options: o}
	if r != nil {
		in.Refs = []types.ResourceRef{*r}
		in.PinnedEndpoint = pin(*r)
		in.Validate = func() error { return validateRef(*r, "volume_type") }
	}
	return core.DoJSON(t.client.runtime, ctx, operation("types.get", "GET"), in, func(raw []byte, ep types.EndpointBinding) (core.Decoded[VolumeType], error) {
		var body struct {
			VolumeType json.RawMessage `json:"volume_type"`
		}
		if json.Unmarshal(raw, &body) != nil {
			return core.Decoded[VolumeType]{}, types.NewError(types.ProtocolError)
		}
		v, e := decodeType(body.VolumeType, ep, id)
		return core.Decoded[VolumeType]{Data: v}, e
	})
}

type Empty struct{}

func emptyDecoder(_ []byte, _ types.EndpointBinding) (core.Decoded[Empty], error) {
	return core.Decoded[Empty]{}, nil
}
