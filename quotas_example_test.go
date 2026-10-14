package selectelsdk_test

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/quotas"
	"github.com/festy23/selectel-sdk/types"
)

// The configured client and authorization to change quotas belong to the caller.
func ExampleRegionalClient_Quotas() {
	inspect := func(ctx context.Context, c *sdk.Client) error {
		project, err := c.Project("explicit-project-id")
		if err != nil {
			return err
		}
		region, err := project.Location("spb", "ru-1")
		if err != nil {
			return err
		}
		result, err := region.Quotas().V1.Get(ctx, quotas.Filters{Resources: []string{"compute_cores"}}, types.CallOptions{})
		_ = result.Data // Retain partial Data when err is incomplete.
		return err
	}
	_ = inspect
}
