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

type ClusterMaintenanceListing struct {
	pagination.Collection[ClusterMntEvent]
	Envelopes []ListClusterMntEventsResponse `json:"envelopes,omitempty"`
}

func (c *Client) ListClusterMaintenanceEvents(ctx context.Context, ref types.ResourceRef, options pagination.Options) (ClusterMaintenanceListing, error) {
	result := ClusterMaintenanceListing{}
	if e := ref.Validate(); e != nil {
		return result, e
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return result, types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "cluster" || ref.Parent != nil {
		return result, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{ID: "cloud_1c.v2.clusters_maintenance.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return result, e
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return result, types.NewError(types.EndpointUnavailable)
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, ref)
	if e != nil {
		return result, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[ClusterMntEvent], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID) + "/maintenance", Query: query, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListClusterMntEventsResponse], error) {
			var v ListClusterMntEventsResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Events == nil {
				return core.Decoded[ListClusterMntEventsResponse]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range v.Events {
				row := &v.Events[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListClusterMntEventsResponse]{}, types.NewError(types.ProtocolError)
				}
				if row.ProjectID != nil && *row.ProjectID != c.scope.ProjectID || row.Spec != nil && row.Spec.ClusterID != nil && *row.Spec.ClusterID != ref.ID {
					return core.Decoded[ListClusterMntEventsResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "cluster_maintenance_event", *row.ID)
				parent := ref
				row.Ref.Parent = &parent
			}
			return core.Decoded[ListClusterMntEventsResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[ClusterMntEvent]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = out.Data.Events
		out.Data.Events = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	iterator, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = iterator.Collect(ctx)
	return result, e
}
