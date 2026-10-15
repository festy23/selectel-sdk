package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
)

type ClusterListing struct {
	pagination.Collection[Cluster]
	Envelopes []ListClustersResponse `json:"envelopes,omitempty"`
}

// ListClusters preserves bounded pages and provider cursor metadata. The pinned
// source has no terminal sentinel contract; it never infers completion from 0,
// an omitted cursor or an empty page.
func (c *Client) ListClusters(ctx context.Context, options pagination.Options) (ClusterListing, error) {
	result := ClusterListing{}
	op := core.Operation{ID: "cloud_1c.v2.clusters.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return result, err
	}
	plan, err := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, nil)
	if err != nil {
		return result, err
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[Cluster], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters", Query: query}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListClustersResponse], error) {
			var v ListClustersResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Clusters == nil {
				return core.Decoded[ListClustersResponse]{}, types.NewError(types.ProtocolError)
			}
			if v.Cursor != nil {
				for _, field := range []*string{v.Cursor.Next, v.Cursor.Total} {
					if field != nil {
						if _, e := strconv.ParseUint(*field, 10, 64); e != nil {
							return core.Decoded[ListClustersResponse]{}, types.NewError(types.ProtocolError)
						}
					}
				}
			}
			seen := map[string]bool{}
			for i := range v.Clusters {
				row := &v.Clusters[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListClustersResponse]{}, types.NewError(types.ProtocolError)
				}
				if row.ProjectID != nil && *row.ProjectID != c.scope.ProjectID {
					return core.Decoded[ListClustersResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "cluster", *row.ID)
			}
			return core.Decoded[ListClustersResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[Cluster]{Metadata: out.Metadata, Next: input}
		if err != nil {
			return page, err
		}
		page.Items = out.Data.Clusters
		out.Data.Clusters = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	iterator, err := pagination.New(plan, options, fetch)
	if err != nil {
		return result, err
	}
	result.Collection, err = iterator.Collect(ctx)
	return result, err
}
