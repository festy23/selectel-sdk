package certificates

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) List(options pagination.Options) (*pagination.Iterator[Certificate], error) {
	op, e := c.operation("certificates.user_v1.certificates.list")
	if e != nil {
		return nil, e
	}
	plan, e := c.runtime.PagePlan(op.ID, op.APIGeneration, c.scope, op.EndpointID, struct {
		EndpointURL string `json:"endpoint_url"`
	}{EndpointURL: op.FixedEndpoint})
	if e != nil {
		return nil, e
	}
	plan.FixedSize = true
	return pagination.New(plan, options, func(ctx context.Context, wire string, _ uint) (pagination.SourcePage[Certificate], error) {
		if wire != "" {
			return pagination.SourcePage[Certificate]{}, types.NewError(types.InvalidArgument)
		}
		items := []Certificate{}
		result, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/certs", PinnedEndpoint: &types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: op.APIGeneration, Audience: "selectel"}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Certificate], error) {
			var rows []json.RawMessage
			if json.Unmarshal(raw, &rows) != nil || rows == nil {
				return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for _, row := range rows {
				var item Certificate
				if json.Unmarshal(row, &item) != nil || item.ID == nil || *item.ID == "" || seen[*item.ID] {
					return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
				}
				item.Ref = *core.Ref(ep, "certificates", "user_certificate", *item.ID)
				if c.valid(item.Ref) != nil {
					return core.Decoded[[]Certificate]{}, types.NewError(types.ProtocolError)
				}
				seen[*item.ID] = true
				items = append(items, item)
			}
			return core.Decoded[[]Certificate]{Data: items}, nil
		})
		return pagination.SourcePage[Certificate]{Items: items, Done: e == nil, Metadata: result.Metadata}, e
	})
}
