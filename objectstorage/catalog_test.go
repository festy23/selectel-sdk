package objectstorage_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/types"
	"testing"
)

func TestManagementCatalogCoversThirteenSourceOperationsHonestly(t *testing.T) {
	c := catalog.Default()
	ops, e := c.List(catalog.Filter{Service: "object_storage"})
	if e != nil || len(ops) != 13 {
		t.Fatal(e, len(ops))
	}
	partial := 0
	for _, op := range ops {
		if op.Auth.Scheme != types.IAMProject || len(op.ScopeRequirements) != 4 || op.Verification.Level == "live" {
			t.Fatal(op.OperationID)
		}
		if op.Implementation.Status == "partial" {
			partial++
			if len(op.ParameterSupport.ExcludedVariants) == 0 {
				t.Fatal("partial without variants")
			}
		} else if op.Implementation.Status != "implemented" {
			t.Fatal(op.OperationID)
		}
	}
	if partial != 2 || c.Summary().InventoryComplete || c.Summary().CoverageRatio != nil {
		t.Fatal(partial)
	}
}
