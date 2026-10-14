// This example performs no requests until the caller supplies credentials,
// account scope and an explicitly selected Billing report endpoint.
package main

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/billing"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func report(ctx context.Context, source credentials.Source, accountID string, endpoint types.EndpointBinding, year, month int) (types.Result[billing.DetailedReportResponse], error) {
	scope := types.Scope{AccountID: accountID}
	client, err := sdk.New(sdk.Config{AccountID: accountID, Credentials: source, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "billing", APIGeneration: "v1", Scope: scope}: endpoint}})
	if err != nil {
		return types.Result[billing.DetailedReportResponse]{}, err
	}
	return client.Account().Billing().DetailedReport(ctx, billing.ReportRequest{Year: year, Month: month}, types.CallOptions{})
}
func main() {}
