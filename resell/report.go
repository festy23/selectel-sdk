package resell

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"time"
)

// TrafficReportRequest uses the provider's calendar dates without timezone conversion.
type TrafficReportRequest struct{ StartDate, EndDate string }

// TrafficReport returns bounded CSV bytes protected from ordinary output.
// Reveal or CopyTo explicitly directs the report to caller-controlled output.
func (c *Client) TrafficReport(ctx context.Context, request TrafficReportRequest, options types.CallOptions) (types.Result[types.ProtectedValue], error) {
	op := core.Operation{ID: "projects.v2.traffic_report.list", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		start, e := time.Parse("2006-01-02", request.StartDate)
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		end, e := time.Parse("2006-01-02", request.EndDate)
		if e != nil || end.Before(start) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[types.ProtectedValue], error) {
		return core.Decoded[types.ProtectedValue]{Data: types.Protect(raw)}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/traffic/report", Query: url.Values{"start_date": {request.StartDate}, "end_date": {request.EndDate}}, Options: options, Validate: validate}, decode)
}

// PublishedSchema reads the published Resell schema with bounded buffering.
// The arbitrary response is protected and never interpreted as SDK coverage.
func (c *Client) PublishedSchema(ctx context.Context, options types.CallOptions) (types.Result[types.ProtectedJSON], error) {
	op := core.Operation{ID: "projects.v2.swagger_doc_json.list", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[types.ProtectedJSON], error) {
		var value types.ProtectedJSON
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[types.ProtectedJSON]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[types.ProtectedJSON]{Data: value}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/swagger/doc.json", Options: options, Validate: validate}, decode)
}
