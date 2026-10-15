package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) GetClusterMetrics(ctx context.Context, cluster types.ResourceRef, span TimeRange, options types.CallOptions) (types.Result[GetClusterMetricsResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_metrics.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	query := url.Values{}
	if span.Start != nil {
		query.Set("timeRange.start", span.Start.UTC().Format(time.RFC3339Nano))
	}
	if span.End != nil {
		query.Set("timeRange.end", span.End.UTC().Format(time.RFC3339Nano))
	}
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/metrics", Query: query, Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterAnchor(cluster, op); e != nil {
			return e
		}
		if span.Start != nil && span.End != nil && span.Start.After(*span.End) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetClusterMetricsResponse], error) {
		var v GetClusterMetricsResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Metrics == nil {
			return core.Decoded[GetClusterMetricsResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetClusterMetricsResponse]{Data: v}, nil
	})
}
