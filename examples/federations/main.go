// Package main illustrates caller-configured, bounded federation reads.
package main

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/federations"
	"github.com/festy23/selectel-sdk/pagination"
)

// SAML returns the bounded collection together with any incomplete/error state.
// The caller supplies the account and CredentialSource when constructing client.
func SAML(ctx context.Context, client *sdk.Client) (pagination.Collection[federations.SAMLFederation], error) {
	return client.Account().Federations().V1.SAML.Collect(ctx, pagination.Options{MaxItems: 100, MaxPages: 1})
}
func main() {}
