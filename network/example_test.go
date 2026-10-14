package network_test

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/network"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

// c is constructed with explicit Config credentials/account and normal runtime.
func ExampleClient() {
	inspect := func(ctx context.Context, c *sdk.Client) error {
		project, e := c.Project("project-id")
		if e != nil {
			return e
		}
		location, e := project.Location("ru-1", "ru-1a")
		if e != nil {
			return e
		}
		n := location.Network()
		visible, e := n.Networks.Collect(ctx, network.NetworkFilters{}, pagination.Options{MaxItems: 100, MaxPages: 10})
		if e != nil {
			return e
		}
		if len(visible.Items) == 0 {
			return nil
		}
		parent := visible.Items[0].Ref
		four, _ := types.ParseInteger("4")
		cidr := "10.13.0.0/24"
		result, e := n.Subnets.Create(ctx, network.SubnetCreateRequest{Network: *parent, Spec: network.SubnetCreateSpec{CIDR: &cidr, IPVersion: &four}}, types.CallOptions{})
		_ = result.Receipt // persist even when e is non-nil; never replay unknown outcome.
		return e
	}
	_ = inspect
}
