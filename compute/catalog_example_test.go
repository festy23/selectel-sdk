package compute_test

import (
	"context"
	"github.com/festy23/selectel-sdk/compute"
	"github.com/festy23/selectel-sdk/types"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// The caller supplies a configured, authorized project/location client and
// identifiers selected from the provider's flavor/image catalogs.
func ExampleServers_CreateWithResult() {
	launch := func(ctx context.Context, c *compute.Client, imageID, flavorID string) (types.Result[compute.CreateResult], error) {
		reservation := true
		return c.Servers.CreateWithResult(ctx, compute.CreateRequest{Microversion: "2.93", ReturnReservationID: &reservation, Options: servers.CreateOpts{Name: "batch", ImageRef: imageID, FlavorRef: flavorID, Networks: "none", Min: 2, Max: 3}})
	}
	// The caller retains both Result.Receipt and err. A reservation is not a wait
	// handle, and an unknown outcome never authorizes replay.
	_ = launch
}
