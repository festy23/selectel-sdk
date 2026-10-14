// Package main illustrates bounded account IAM reads. It performs no I/O by default.
package main

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/iam"
	"github.com/festy23/selectel-sdk/pagination"
)

// Users accepts an explicitly configured client, keeping credential loading and
// account selection at the caller's boundary. An incomplete collection must be
// retained together with its error and continuation.
func Users(ctx context.Context, client *sdk.Client) (pagination.Collection[iam.User], error) {
	return client.Account().IAM().Users.Collect(ctx, pagination.Options{MaxItems: 100, MaxPages: 1})
}
func main() {}
