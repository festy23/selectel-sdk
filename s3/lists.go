package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/internal/endpoints"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type BucketListOptions struct {
	NativePagination  bool
	Prefix, Region    *string
	ContinuationToken *types.ProtectedValue
	types.CallOptions
}
type ObjectListOptions struct {
	Prefix, Delimiter, StartAfter *string
	Marker, ContinuationToken     *types.ProtectedValue
	FetchOwner                    *bool
	EncodingTypeURL               bool
	types.CallOptions
}
type sourcePage[T any] struct {
	Items []T
	Next  *types.ProtectedValue
	Count *types.Integer
}

func list[T any](s *Client, id, path string, ref *types.ResourceRef, q url.Values, initial *types.ProtectedValue, token string, o pagination.Options, callOptions types.CallOptions, decode func(io.Reader, types.EndpointBinding) (sourcePage[T], error)) (*pagination.Iterator[T], error) {
	if e := s.scopeValid(); e != nil {
		return nil, e
	}
	if ref != nil {
		if e := s.valid(*ref, "bucket_name"); e != nil {
			return nil, e
		}
	}
	operation, e := s.callOp(id, "GET", 200)
	if e != nil {
		return nil, e
	}
	if ref != nil {
		operation = addressOperation(operation, *ref)
	}
	boundEndpoint := types.EndpointBinding{ID: operation.EndpointID, URL: operation.FixedEndpoint, Scope: s.scope, APIGeneration: generation, Audience: "selectel"}
	if ref != nil && virtualRef(*ref) {
		var e error
		boundEndpoint, e = endpoints.S3VirtualHosted(boundEndpoint, ref.ID)
		if e != nil {
			return nil, e
		}
	}
	fingerprint := sha256.Sum256([]byte(boundEndpoint.URL + "\n" + path + "\n" + q.Encode() + "\n" + stringValue(initial)))
	plan, e := s.runtime.PagePlan(operation.ID, generation, s.scope, operation.EndpointID, hex.EncodeToString(fingerprint[:]))
	if e != nil {
		return nil, e
	}
	plan.EndpointID = boundEndpoint.ID
	plan.WireMax = 1000
	return pagination.New(plan, o, func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		query := url.Values{}
		for k, v := range q {
			query[k] = append([]string(nil), v...)
		}
		if cursor == "" {
			cursor = stringValue(initial)
		}
		if cursor != "" {
			query.Set(token, cursor)
		}
		if id != "buckets.list" {
			query.Set("max-keys", strconv.FormatUint(uint64(size), 10))
		} else if q.Has("max-buckets") {
			query.Set("max-buckets", strconv.FormatUint(uint64(size), 10))
		}
		req := core.Request{Scope: s.scope, Path: path, Query: query, Options: callOptions, Validate: s.scopeValid}
		if ref != nil {
			req.Refs = []types.ResourceRef{*ref}
			req.PinnedEndpoint = pin(*ref)
			req.Validate = func() error { return s.valid(*ref, "bucket_name") }
		}
		v, e := core.DoResponse(s.runtime, ctx, operation, req, func(_ context.Context, b io.Reader, ep types.EndpointBinding, _ int, h http.Header) (core.Decoded[sourcePage[T]], error) {
			if e := s.headerLimit(h); e != nil {
				return core.Decoded[sourcePage[T]]{}, e
			}
			page, e := decode(b, ep)
			if page.Next != nil && (page.Next.Empty() || stringValue(page.Next) == cursor) {
				return core.Decoded[sourcePage[T]]{}, types.NewError(types.ProtocolError)
			}
			return core.Decoded[sourcePage[T]]{Data: page}, e
		})
		return pagination.SourcePage[T]{Items: v.Data.Items, SensitiveNext: v.Data.Next, Done: v.Data.Next == nil, Metadata: v.Metadata, Provider: pagination.ProviderMetadata{ReportedCount: v.Data.Count}}, e
	})
}
func stringValue(v *types.ProtectedValue) string {
	if v == nil {
		return ""
	}
	return string(v.Reveal())
}
func (c *Buckets) Iterate(in BucketListOptions, o pagination.Options) (*pagination.Iterator[BucketSummary], error) {
	q := url.Values{}
	if !in.NativePagination && (in.Prefix != nil || in.Region != nil || in.ContinuationToken != nil) {
		return nil, types.NewError(types.InvalidArgument)
	}
	if in.NativePagination {
		q.Set("max-buckets", "1")
		if in.Prefix != nil {
			q.Set("prefix", *in.Prefix)
		}
		if in.Region != nil {
			q.Set("bucket-region", *in.Region)
		}
	}
	s := c.client
	return list(s, "buckets.list", "/", nil, q, in.ContinuationToken, "continuation-token", o, in.CallOptions, func(b io.Reader, ep types.EndpointBinding) (sourcePage[BucketSummary], error) {
		var body struct {
			XMLName xml.Name `xml:"ListAllMyBucketsResult"`
			Buckets []struct {
				Name         *string `xml:"Name"`
				CreationDate *string `xml:"CreationDate"`
				Region       *string `xml:"BucketRegion"`
				ARN          *string `xml:"BucketArn"`
			} `xml:"Buckets>Bucket"`
			Token *string `xml:"ContinuationToken"`
		}
		out := sourcePage[BucketSummary]{}
		if e := s.readXML(b, &body); e != nil {
			return out, e
		}
		for _, v := range body.Buckets {
			if v.Name == nil || !bucketName(*v.Name) {
				return out, types.NewError(types.ProtocolError)
			}
			r, e := s.bucketRef(ep, *v.Name)
			if e != nil {
				return out, e
			}
			out.Items = append(out.Items, BucketSummary{Name: *v.Name, CreationDate: v.CreationDate, Region: v.Region, ARN: v.ARN, Ref: &r})
		}
		if body.Token != nil && *body.Token != "" {
			p := types.Protect([]byte(*body.Token))
			out.Next = &p
		}
		return out, nil
	})
}
func (c *Buckets) ListPage(ctx context.Context, in BucketListOptions, o pagination.Options) (pagination.Page[BucketSummary], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Page[BucketSummary]{}, e
	}
	return it.Next(ctx)
}
func (c *Buckets) Collect(ctx context.Context, in BucketListOptions, o pagination.Options) (pagination.Collection[BucketSummary], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Collection[BucketSummary]{}, e
	}
	return it.Collect(ctx)
}
func (c *Objects) iterate(r types.ResourceRef, in ObjectListOptions, o pagination.Options, v2 bool) (*pagination.Iterator[ListEntry], error) {
	q := url.Values{}
	id, token, initial := "objects.list_v1", "marker", in.Marker
	if v2 {
		id, token, initial = "objects.list_v2", "continuation-token", in.ContinuationToken
		q.Set("list-type", "2")
		if in.Marker != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		if in.StartAfter != nil {
			q.Set("start-after", *in.StartAfter)
		}
		if in.FetchOwner != nil {
			q.Set("fetch-owner", strconv.FormatBool(*in.FetchOwner))
		}
	} else if in.ContinuationToken != nil || in.StartAfter != nil || in.FetchOwner != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if in.Prefix != nil {
		q.Set("prefix", *in.Prefix)
	}
	if in.Delimiter != nil {
		q.Set("delimiter", *in.Delimiter)
	}
	if in.EncodingTypeURL {
		q.Set("encoding-type", "url")
	}
	s := c.client
	return list(s, id, bucketPath(r), &r, q, initial, token, o, in.CallOptions, func(b io.Reader, ep types.EndpointBinding) (sourcePage[ListEntry], error) {
		var body struct {
			XMLName   xml.Name    `xml:"ListBucketResult"`
			Name      *string     `xml:"Name"`
			Truncated *bool       `xml:"IsTruncated"`
			Encoding  string      `xml:"EncodingType"`
			Count     *xmlInteger `xml:"KeyCount"`
			NextV1    *string     `xml:"NextMarker"`
			NextV2    *string     `xml:"NextContinuationToken"`
			Contents  []struct {
				Key          *string     `xml:"Key"`
				Size         *xmlInteger `xml:"Size"`
				ETag         *string     `xml:"ETag"`
				LastModified *string     `xml:"LastModified"`
				StorageClass *string     `xml:"StorageClass"`
				Owner        *Owner      `xml:"Owner"`
			} `xml:"Contents"`
			Prefixes []struct {
				Prefix *string `xml:"Prefix"`
			} `xml:"CommonPrefixes"`
		}
		out := sourcePage[ListEntry]{}
		if e := s.readXML(b, &body); e != nil {
			return out, e
		}
		if body.Name == nil || *body.Name != r.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		if body.Truncated == nil {
			return out, types.NewError(types.ProtocolError)
		}
		unescape := func(v string) (string, error) {
			if body.Encoding == "url" {
				x, e := url.PathUnescape(v)
				if e != nil {
					return "", types.NewError(types.ProtocolError)
				}
				return x, nil
			}
			if body.Encoding != "" {
				return "", types.NewError(types.ProtocolError)
			}
			return v, nil
		}
		last := ""
		for _, v := range body.Contents {
			if v.Key == nil {
				return out, types.NewError(types.ProtocolError)
			}
			key, e := unescape(*v.Key)
			if e != nil {
				return out, e
			}
			object, e := s.Object(r, key)
			if e != nil {
				return out, types.NewError(types.ProtocolError)
			}
			out.Items = append(out.Items, ListEntry{Object: &ObjectSummary{Key: key, Size: integerValue(v.Size), ETag: v.ETag, LastModified: v.LastModified, StorageClass: v.StorageClass, Owner: v.Owner, Ref: &object}})
			last = key
		}
		for _, v := range body.Prefixes {
			if v.Prefix == nil {
				return out, types.NewError(types.ProtocolError)
			}
			prefix, e := unescape(*v.Prefix)
			if e != nil {
				return out, e
			}
			p := types.Protect([]byte(prefix))
			out.Items = append(out.Items, ListEntry{CommonPrefix: &p})
		}
		out.Count = integerValue(body.Count)
		if *body.Truncated {
			var next string
			if v2 {
				if body.NextV2 == nil || *body.NextV2 == "" {
					return out, types.NewError(types.ProtocolError)
				}
				next = *body.NextV2
			} else if body.NextV1 != nil && *body.NextV1 != "" {
				var e error
				next, e = unescape(*body.NextV1)
				if e != nil {
					return out, e
				}
			} else {
				if last == "" {
					return out, types.NewError(types.ProtocolError)
				}
				next = last
			}
			p := types.Protect([]byte(next))
			out.Next = &p
		}
		return out, nil
	})
}
func (c *Objects) Iterate(r types.ResourceRef, in ObjectListOptions, o pagination.Options) (*pagination.Iterator[ListEntry], error) {
	return c.iterate(r, in, o, true)
}
func (c *Objects) IterateV1(r types.ResourceRef, in ObjectListOptions, o pagination.Options) (*pagination.Iterator[ListEntry], error) {
	return c.iterate(r, in, o, false)
}
func (c *Objects) ListPage(ctx context.Context, r types.ResourceRef, in ObjectListOptions, o pagination.Options) (pagination.Page[ListEntry], error) {
	it, e := c.Iterate(r, in, o)
	if e != nil {
		return pagination.Page[ListEntry]{}, e
	}
	return it.Next(ctx)
}
func (c *Objects) ListPageV1(ctx context.Context, r types.ResourceRef, in ObjectListOptions, o pagination.Options) (pagination.Page[ListEntry], error) {
	it, e := c.IterateV1(r, in, o)
	if e != nil {
		return pagination.Page[ListEntry]{}, e
	}
	return it.Next(ctx)
}
func (c *Objects) Collect(ctx context.Context, r types.ResourceRef, in ObjectListOptions, o pagination.Options) (pagination.Collection[ListEntry], error) {
	it, e := c.Iterate(r, in, o)
	if e != nil {
		return pagination.Collection[ListEntry]{}, e
	}
	return it.Collect(ctx)
}
func (c *Objects) CollectV1(ctx context.Context, r types.ResourceRef, in ObjectListOptions, o pagination.Options) (pagination.Collection[ListEntry], error) {
	it, e := c.IterateV1(r, in, o)
	if e != nil {
		return pagination.Collection[ListEntry]{}, e
	}
	return it.Collect(ctx)
}

type xmlInteger struct{ value types.Integer }

func (v *xmlInteger) UnmarshalText(raw []byte) error {
	i, e := types.ParseInteger(string(raw))
	if e != nil || len(raw) > 0 && raw[0] == '-' {
		return types.NewError(types.ProtocolError)
	}
	v.value = i
	return nil
}
func integerValue(v *xmlInteger) *types.Integer {
	if v == nil {
		return nil
	}
	out := v.value
	return &out
}
