package network_test

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestNetworkCatalogCoversOnlyConfirmedCoreSubset(t *testing.T) {
	c := catalog.Default()
	all, err := c.List(catalog.Filter{Service: "network", APIGeneration: "v2"})
	if err != nil || len(all) != 15 {
		t.Fatalf("network subset %d %v", len(all), err)
	}
	for _, resource := range []string{"networks", "subnets", "ports"} {
		for _, action := range []string{"create", "list", "get", "update", "delete"} {
			id := "network.v2." + resource + "." + action
			v, e := c.Describe(id)
			if e != nil || v.Implementation.Status != "implemented" || v.Service != "network" || string(v.Auth.Scheme) != "iam_project" || v.Verification.Level != "offline" && v.Verification.Level != "none" {
				t.Fatalf("binding %s %+v %v", id, v, e)
			}
		}
	}
	if c.Summary().InventoryComplete || c.Summary().CoverageRatio != nil {
		t.Fatal("invented complete inventory")
	}
}

func TestNetworkHTTP400ContractMatchesSharedRuntime(t *testing.T) {
	c := catalog.Default()
	for _, resource := range []string{"networks", "subnets", "ports"} {
		for _, action := range []string{"create", "list", "get", "update", "delete"} {
			raw, e := c.Artifact("contracts/network.v2." + resource + "." + action + ".json")
			if e != nil {
				t.Fatal(e)
			}
			var contract struct {
				Errors map[string]string `json:"errors"`
			}
			if json.Unmarshal(raw, &contract) != nil || contract.Errors["400"] != "provider_error" {
				t.Fatalf("400 contract %s/%s: %s", resource, action, raw)
			}
		}
	}
}
