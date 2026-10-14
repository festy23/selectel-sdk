package cdn

import (
	"context"
	"encoding/json"
	"net/url"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type DeliveryOptions struct{ CDNDomain *types.ProtectedValue }

func (c *Client) ListOriginNodes(ctx context.Context, options pagination.Options) (Listing[Node, NodesResponse], error) {
	return c.nodes(ctx, "cdn.v3.origin_nodes.list", "/nodes/ip2origin", nil, options)
}
func (c *Client) ListDeliveryNodes(ctx context.Context, spec DeliveryOptions, options pagination.Options) (Listing[Node, NodesResponse], error) {
	q := url.Values{}
	if spec.CDNDomain != nil {
		domain := spec.CDNDomain.Reveal()
		if !utf8.Valid(domain) {
			return Listing[Node, NodesResponse]{}, types.NewError(types.InvalidArgument)
		}
		q.Set("cdn-domain", string(domain))
	}
	return c.nodes(ctx, "cdn.v3.delivery_nodes.list", "/nodes/ip2user", q, options)
}
func (c *Client) nodes(ctx context.Context, id, path string, q url.Values, options pagination.Options) (Listing[Node, NodesResponse], error) {
	envelopes := []NodesResponse{}
	collection, e := collectFixed(c, ctx, id, path, q, options, func(raw []byte, _ types.EndpointBinding) (core.Decoded[[]Node], error) {
		var envelope NodesResponse
		if json.Unmarshal(raw, &envelope) != nil {
			return core.Decoded[[]Node]{}, types.NewError(types.ProtocolError)
		}
		data := envelope.Data
		envelope.Data = nil
		envelopes = append(envelopes, envelope)
		if data == nil {
			return core.Decoded[[]Node]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[[]Node]{Data: *data}, nil
	})
	return Listing[Node, NodesResponse]{Collection: collection, Envelopes: envelopes}, e
}
