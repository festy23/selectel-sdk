package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/types"
)

// This example is compiled, but has no Output directive: tests never provision
// cloud resources. Supply explicit profile, scope and provider IDs before use.
func Example_datastoreProvisioning() {
	source, err := credentials.FromProfile("profiles.json", "selected-profile")
	if err != nil {
		return
	}
	client, err := sdk.New(sdk.Config{AccountID: "account-id", Credentials: source})
	if err != nil {
		return
	}
	project, err := client.Project("project-id")
	if err != nil {
		return
	}
	location, err := project.Location("", "ru-1")
	if err != nil {
		return
	}
	name, typeID, subnetID, flavorID := "app-db", "datastore-type-id", "subnet-id", "flavor-id"
	nodes, err := types.ParseInteger("1")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	created, err := location.Databases().V1.Datastores.Create(ctx, databases.CreateRequest{
		Name: &name, TypeID: &typeID, SubnetID: &subnetID, FlavorID: &flavorID, NodeCount: &nodes,
	}, types.CallOptions{})
	// Persist this receipt in caller-owned storage even when Create returns an error.
	receiptJSON, encodeErr := json.Marshal(created.Receipt)
	if encodeErr != nil {
		return
	}
	fmt.Println(string(receiptJSON))
	if err != nil {
		return
	}
	handle, err := client.Resume(*created.Receipt)
	if err != nil {
		return
	}
	waited, err := handle.Wait(ctx, "datastore.active")
	if err != nil {
		fmt.Println("wait error:", types.ErrorCode(err), "creation outcome:", waited.Receipt.Outcome)
		return
	}
	fmt.Println("provider state:", waited.Last.ProviderState)
}
