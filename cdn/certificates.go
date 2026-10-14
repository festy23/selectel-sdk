package cdn

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func logicalStatus(status *types.Integer, model string) error {
	if status == nil {
		return types.NewError(types.ProtocolError)
	}
	if status.String() == "0" {
		return nil
	}
	code := responseCodeClasses[model][status.String()]
	if code == "" {
		code = types.OperationFailed
	}
	e := types.NewError(code)
	if len(status.String()) <= 64 {
		e.ProviderCode = status.String()
	}
	return e
}
func decodeCertificateChange[T any](expected string, setRef func(*T, *types.ResourceRef), status func(*T) *types.Integer) core.Decoder[T] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[T], error) {
		out := core.Decoded[T]{}
		id := expected
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		var identityErr error
		if data, ok := fields["data"]; ok && string(data) != "null" {
			var value map[string]json.RawMessage
			if json.Unmarshal(data, &value) != nil || value == nil {
				identityErr = types.NewError(types.ProtocolError)
			} else if seq, ok := value["seq"]; ok {
				var sequence types.Integer
				if json.Unmarshal(seq, &sequence) != nil {
					identityErr = types.NewError(types.ProtocolError)
				} else if expected != "" && sequence.String() != expected {
					identityErr = types.NewError(types.ScopeMismatch)
				} else {
					id = sequence.String()
				}
			}
		}
		var ref *types.ResourceRef
		if id != "" {
			ref = core.Ref(ep, "cdn", "certificate", id)
			out.Refs = []types.ResourceRef{*ref}
		}
		if identityErr != nil {
			return out, identityErr
		}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		setRef(&out.Data, ref)
		if e := logicalStatus(status(&out.Data), reflect.TypeOf(out.Data).Name()); e != nil {
			return out, e
		}
		if ref == nil {
			return out, types.NewError(types.ProtocolError)
		}
		return out, nil
	}
}
func (c *Client) CreateCertificate(ctx context.Context, spec CreateCertificateRequest, o types.CallOptions) (types.Result[CreateCertificateResponse], error) {
	op, bindErr := c.operation("cdn.v3.certificates.create", "POST")
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certificate", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return encodeErr
	}}, decodeCertificateChange("", func(v *CreateCertificateResponse, r *types.ResourceRef) { v.Ref = r }, func(v *CreateCertificateResponse) *types.Integer { return v.Status }))
}

type CertificateOptions struct{ Details *bool }

func (c *Client) Certificate(sequence types.Integer) (types.ResourceRef, error) {
	n, e := types.ParseInteger(sequence.String())
	if e != nil {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("cdn.v3.certificates.get", "GET")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: "v3", Scope: c.scope}, "cdn", "certificate", n.String()), nil
}
func (c *Client) GetCertificate(ctx context.Context, ref types.ResourceRef, spec CertificateOptions, o types.CallOptions) (types.Result[GetCertificateResponse], error) {
	op, bindErr := c.operation("cdn.v3.certificates.get", "GET")
	q := url.Values{}
	if spec.Details != nil {
		q.Set("details", strconv.FormatBool(*spec.Details))
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certificate/" + url.PathEscape(ref.ID), Query: q, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "certificate")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetCertificateResponse], error) {
		var out GetCertificateResponse
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[GetCertificateResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := logicalStatus(out.Status, "GetCertificateResponse"); e != nil {
			return core.Decoded[GetCertificateResponse]{Data: out}, e
		}
		if out.Data == nil {
			return core.Decoded[GetCertificateResponse]{}, types.NewError(types.ProtocolError)
		}
		for i := range *out.Data {
			cert := &(*out.Data)[i]
			if cert.Seq != nil && cert.Seq.String() != ref.ID {
				return core.Decoded[GetCertificateResponse]{}, types.NewError(types.ScopeMismatch)
			}
			known := ref
			cert.Ref = &known
		}
		return core.Decoded[GetCertificateResponse]{Data: out}, nil
	})
}
func (c *Client) UpdateCertificate(ctx context.Context, ref types.ResourceRef, spec UpdateCertificateRequest, o types.CallOptions) (types.Result[UpdateCertificateResponse], error) {
	op, bindErr := c.operation("cdn.v3.certificates.update", "PATCH")
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certificate/" + url.PathEscape(ref.ID), Body: body, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if encodeErr != nil {
			return encodeErr
		}
		return c.refError(ref, "certificate")
	}}, decodeCertificateChange(ref.ID, func(v *UpdateCertificateResponse, r *types.ResourceRef) { v.Ref = r }, func(v *UpdateCertificateResponse) *types.Integer { return v.Status }))
}
func (c *Client) DeleteCertificate(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[DeleteCertificateResponse], error) {
	op, bindErr := c.operation("cdn.v3.certificates.delete", "DELETE")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certificate/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "certificate")
	}}, decodeCertificateChange(ref.ID, func(v *DeleteCertificateResponse, r *types.ResourceRef) { v.Ref = r }, func(v *DeleteCertificateResponse) *types.Integer { return v.Status }))
}

func (c *Client) ListCertificates(ctx context.Context, options pagination.Options) (Listing[CertificateInfo, ListCertificatesResponse], error) {
	envelopes := []ListCertificatesResponse{}
	collection, e := collectFixed(c, ctx, "cdn.v3.certificates.list", "/certificate", nil, options, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]CertificateInfo], error) {
		var envelope ListCertificatesResponse
		if json.Unmarshal(raw, &envelope) != nil {
			return core.Decoded[[]CertificateInfo]{}, types.NewError(types.ProtocolError)
		}
		items := envelope.Data
		envelope.Data = nil
		envelopes = append(envelopes, envelope)
		if e := logicalStatus(envelope.Status, "ListCertificatesResponse"); e != nil {
			return core.Decoded[[]CertificateInfo]{}, e
		}
		if items == nil {
			return core.Decoded[[]CertificateInfo]{}, types.NewError(types.ProtocolError)
		}
		for i := range *items {
			if (*items)[i].Seq != nil {
				(*items)[i].Ref = core.Ref(ep, "cdn", "certificate", (*items)[i].Seq.String())
			}
		}
		return core.Decoded[[]CertificateInfo]{Data: *items}, nil
	})
	return Listing[CertificateInfo, ListCertificatesResponse]{Collection: collection, Envelopes: envelopes}, e
}
