// Resell reads require an explicitly supplied CredentialSource and account ID.
package main

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/resell"
	"github.com/festy23/selectel-sdk/types"
)

func accountStatus(ctx context.Context, source credentials.Source, accountID string) (bool, error) {
	client, err := sdk.New(sdk.Config{AccountID: accountID, Credentials: source})
	if err != nil {
		return false, err
	}
	result, err := client.Account().Resell().Account(ctx, types.CallOptions{})
	if err != nil {
		return false, err
	}
	return *result.Data.Enabled, nil
}
func main() {}

// projectSubnetRefs lists Resell allocation identities for an explicit project.
// Its caller must inspect Complete/StopReason if the configured limit is reached.
func projectSubnetRefs(ctx context.Context, source credentials.Source, accountID, projectID string) (pagination.Collection[resell.Subnet], error) {
	client, err := sdk.New(sdk.Config{AccountID: accountID, Credentials: source})
	if err != nil {
		return pagination.Collection[resell.Subnet]{}, err
	}
	project, err := client.Project(projectID)
	if err != nil {
		return pagination.Collection[resell.Subnet]{}, err
	}
	return project.Resell().Subnets.Collect(ctx, resell.AllocationListOptions{}, pagination.Options{MaxItems: 100})
}
