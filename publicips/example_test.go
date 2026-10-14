package publicips_test

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/publicips"
	"github.com/festy23/selectel-sdk/types"
)

// Credentials/account and project/location are selected explicitly by the caller.
func ExampleAllocations() {
	allocate := func(ctx context.Context, c *sdk.Client) error {
		pool := "ru-3"
		quantity, _ := types.ParseInteger("1")
		result, e := c.Account().PublicIPAllocations().Create(ctx, publicips.AllocationCreateRequest{ProjectID: "project-id", Specs: []publicips.AllocationSpec{{Region: &pool, Quantity: &quantity}}}, types.CallOptions{})
		_ = result.Receipt // Persist even on error. Never replay an unknown allocation.
		return e
	}
	_ = allocate
}
