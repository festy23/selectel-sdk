package volumes_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestNineSourceBoundOperationsRemainHonestAboutInventory(t *testing.T) {
	ids := []string{"volumes.create", "volumes.get", "volumes.update", "volumes.delete", "volumes.extend", "volumes.list", "types.get", "types.list", "types.qos_limits"}
	c := catalog.Default()
	rows, e := c.List(catalog.Filter{Service: "volumes"})
	if e != nil || len(rows) != 9 {
		t.Fatalf("volume bindings %d %v", len(rows), e)
	}
	if c.Summary().InventoryComplete || c.Summary().CoverageRatio != nil {
		t.Fatal("inventory completeness overclaimed")
	}
	for _, id := range ids {
		cap, e := c.Describe("volumes.v3." + id)
		if e != nil {
			t.Fatal(e)
		}
		if cap.Implementation.Status != "implemented" || cap.APIGeneration != "v3" || len(cap.ScopeRequirements) != 4 {
			t.Fatalf("%+v", cap)
		}
	}
}
