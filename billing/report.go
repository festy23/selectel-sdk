package billing

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
)

type ReportRequest struct {
	Year, Month int
	Locale      *string
}

// DetailedReport requires an explicit billing/v1 endpoint override: the pinned
// public report source publishes its path but no server/host recipe.
func (c *Client) DetailedReport(ctx context.Context, request ReportRequest, options types.CallOptions) (types.Result[DetailedReportResponse], error) {
	endpoint, endpointErr := c.runtime.ExplicitEndpoint("billing", "v1", c.scope)
	op := core.Operation{ID: "billing.v1.billing_report_by_project_detailed.list", Service: "billing", APIGeneration: "v1", Method: http.MethodGet, Effect: "read", Auth: types.StaticToken, EndpointID: endpoint.ID, FixedEndpoint: endpoint.URL, Success: []int{200}}
	q := url.Values{"year": {strconv.Itoa(request.Year)}, "month": {strconv.Itoa(request.Month)}}
	if request.Locale != nil {
		q.Set("locale", *request.Locale)
	}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		if request.Year < 2020 || request.Year > 2030 || request.Month < 1 || request.Month > 12 || (request.Locale != nil && !member(*request.Locale, "ru", "en")) {
			return types.NewError(types.InvalidArgument)
		}
		return endpointErr
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[DetailedReportResponse], error) {
		var value DetailedReportResponse
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[DetailedReportResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[DetailedReportResponse]{}, e
		}
		if value.Data.AccountID.String() != c.scope.AccountID {
			return core.Decoded[DetailedReportResponse]{}, types.NewError(types.ScopeMismatch)
		}
		return core.Decoded[DetailedReportResponse]{Data: value}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/billing/report/by_project/detailed", Query: q, Options: options, Validate: validate}, decode)
}
