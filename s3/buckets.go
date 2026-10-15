package s3

import (
	"context"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
)

func invoke[T any](s *Client, ctx context.Context, id, method string, r types.ResourceRef, q url.Values, h http.Header, body []byte, status []int, o types.CallOptions, validate func() error, decode core.ResponseDecoder[T]) (types.Result[T], error) {
	op, bind := s.callOp(id, method, status...)
	if h != nil {
		op.Headers = h
	}
	op = addressOperation(op, r)
	path := bucketPath(r)
	kind := "bucket_name"
	if kindMatches(r.Kind, "object_key") {
		path = objectWirePath(r)
		kind = "object_key"
	}
	return core.DoResponse(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: path, Query: q, Body: body, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if kind == "object_key" {
			if e := s.validObject(r); e != nil {
				return e
			}
		} else if e := s.valid(r, kind); e != nil {
			return e
		}
		if validate != nil {
			return validate()
		}
		return nil
	}}, func(call context.Context, b io.Reader, ep types.EndpointBinding, status int, h http.Header) (core.Decoded[T], error) {
		if e := s.headerLimit(h); e != nil {
			return core.Decoded[T]{}, e
		}
		return decode(call, b, ep, status, h)
	})
}
func emptyBody(body io.Reader) error {
	var p [1]byte
	n, e := body.Read(p[:])
	if n != 0 || (e != nil && e != io.EOF) {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
func (s *Client) readXML(body io.Reader, out any) error {
	limit := s.runtime.StructuredLimit()
	data, e := io.ReadAll(io.LimitReader(body, int64(min(limit, uint64(^uint64(0)>>1)-1))+1))
	if e != nil {
		return types.NewError(types.TransportError)
	}
	if uint64(len(data)) > limit {
		return types.NewError(types.ResponseLimit)
	}
	if xml.Unmarshal(data, out) != nil {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
func (c *Buckets) Head(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[BucketInfo], error) {
	return invoke(c.client, ctx, "buckets.head", "HEAD", r, nil, nil, nil, []int{200}, o, nil, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[BucketInfo], error) {
		out := BucketInfo{Ref: &r}
		if s := h.Get("X-Amz-Bucket-Region"); s != "" {
			out.Region = &s
		}
		return core.Decoded[BucketInfo]{Data: out}, emptyBody(b)
	})
}
func (c *Buckets) Location(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[BucketInfo], error) {
	return invoke(c.client, ctx, "buckets.location", "GET", r, url.Values{"location": {""}}, nil, nil, []int{200}, o, nil, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, _ http.Header) (core.Decoded[BucketInfo], error) {
		var v struct {
			XMLName xml.Name `xml:"LocationConstraint"`
			Value   string   `xml:",chardata"`
		}
		e := c.client.readXML(b, &v)
		out := BucketInfo{Ref: &r}
		if e == nil {
			out.Location = &v.Value
		}
		return core.Decoded[BucketInfo]{Data: out}, e
	})
}
func (c *Buckets) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return invoke(c.client, ctx, "buckets.delete", "DELETE", r, nil, nil, nil, []int{204}, o, nil, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, _ http.Header) (core.Decoded[Empty], error) {
		return core.Decoded[Empty]{}, emptyBody(b)
	})
}
