// Package main shows explicit credential-change steps; it performs no I/O by default.
package main

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/iamcredentials"
	"github.com/festy23/selectel-sdk/types"
)

// CreateReplacement returns its independent Result/Receipt. The caller must
// explicitly protect/store/install/test the new key before choosing to revoke old.
func CreateReplacement(ctx context.Context, client *sdk.Client, parent types.ResourceRef, name, project string) (types.Result[iamcredentials.CreatedCredential], error) {
	return client.Account().S3Credentials().V1.ServiceUsers.Create(ctx, parent, iamcredentials.CreateRequest{Name: &name, ProjectID: &project}, types.CallOptions{})
}

// RevokeOld is a separate caller-directed step. Retain its receipt on an error;
// an unknown outcome must be observed by exact metadata if supported, never replayed.
func RevokeOld(ctx context.Context, client *sdk.Client, old types.ResourceRef) (types.Result[struct{}], error) {
	return client.Account().S3Credentials().V1.ServiceUsers.Delete(ctx, old, types.CallOptions{})
}
func main() {}
