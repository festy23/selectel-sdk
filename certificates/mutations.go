package certificates

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func pemWire(in *PEMCertificate) *wirePEM {
	if in == nil {
		return nil
	}
	out := &wirePEM{Certificates: in.Certificates}
	if in.PrivateKey != nil {
		raw := string(in.PrivateKey.Reveal())
		out.PrivateKey = &raw
	}
	return out
}
func (c *Client) Upload(ctx context.Context, in UploadCertificateRequest, o types.CallOptions) (types.Result[Certificate], error) {
	op, bind := c.operation("certificates.user_v1.certificates.upload")
	raw, _ := json.Marshal(wireUpload{Name: in.Name, PEM: pemWire(in.PEM)})
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certs", Body: raw, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if uint64(len(raw)) > c.runtime.StructuredLimit() {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Certificate], error) {
		var identity struct {
			ID *string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		ref := *core.Ref(ep, "certificates", "user_certificate", *identity.ID)
		if c.valid(ref) != nil {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		out := Certificate{Ref: ref}
		decoded := func(e error) (core.Decoded[Certificate], error) {
			return core.Decoded[Certificate]{Data: out, Refs: []types.ResourceRef{ref}}, e
		}
		if json.Unmarshal(raw, &out) != nil {
			return decoded(types.NewError(types.ProtocolError))
		}
		out.Ref = ref
		return decoded(nil)
	})
}

func (c *Client) edit(ctx context.Context, ref types.ResourceRef, id string, body []byte, query url.Values, o types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.operation(id)
	path := "/cert/" + url.PathEscape(ref.ID)
	if strings.HasPrefix(id, "certificates.user_v1.consumers.") {
		path += "/consumers"
	}
	return core.DoResponse[struct{}](c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Query: query, Body: body, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bind != nil {
			return bind
		}
		if e := c.valid(ref); e != nil {
			return e
		}
		if uint64(len(body)) > c.runtime.StructuredLimit() {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(context.Context, io.Reader, types.EndpointBinding, int, http.Header) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *Client) UpdateVersion(ctx context.Context, ref types.ResourceRef, in UpdateCertificateVersionRequest, o types.CallOptions) (types.Result[struct{}], error) {
	raw, _ := json.Marshal(wireVersion{PEM: pemWire(in.PEM)})
	return c.edit(ctx, ref, "certificates.user_v1.versions.create", raw, nil, o)
}
func (c *Client) UpdateName(ctx context.Context, ref types.ResourceRef, in UpdateCertificateRequest, o types.CallOptions) (types.Result[struct{}], error) {
	raw, _ := json.Marshal(in)
	return c.edit(ctx, ref, "certificates.user_v1.name.update", raw, nil, o)
}

func (c *Client) AddConsumers(ctx context.Context, ref types.ResourceRef, in UpdateCertificateConsumerRequest, o types.CallOptions) (types.Result[struct{}], error) {
	raw, _ := json.Marshal(in)
	return c.edit(ctx, ref, "certificates.user_v1.consumers.add", raw, nil, o)
}
func (c *Client) RemoveConsumers(ctx context.Context, ref types.ResourceRef, in UpdateCertificateConsumerRequest, o types.CallOptions) (types.Result[struct{}], error) {
	raw, _ := json.Marshal(in)
	return c.edit(ctx, ref, "certificates.user_v1.consumers.remove", raw, nil, o)
}

func (c *Client) Delete(ctx context.Context, ref types.ResourceRef, in DeleteOptions, o types.CallOptions) (types.Result[struct{}], error) {
	q := url.Values{}
	if in.Force != nil {
		q.Set("force", strconv.FormatBool(*in.Force))
	}
	return c.edit(ctx, ref, "certificates.user_v1.certificates.delete", nil, q, o)
}
