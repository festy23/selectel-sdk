package s3_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/types"
	"testing"
)

func TestS3CatalogRetainsPartialNativeCoverageAndUnknownInventory(t *testing.T) {
	c := catalog.Default()
	ops, e := c.List(catalog.Filter{Service: "s3"})
	if e != nil || len(ops) != 13 {
		t.Fatal(e, len(ops))
	}
	for _, op := range ops {
		if op.Implementation.Status != "partial" || op.Auth.Scheme != types.S3SigV4 || len(op.ParameterSupport.ExcludedVariants) == 0 || op.Verification.Level == "live" {
			t.Fatal(op.OperationID)
		}
		if len(op.ScopeRequirements) != 4 {
			t.Fatal(op.ScopeRequirements)
		}
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil {
		t.Fatal("invented complete coverage")
	}
}
