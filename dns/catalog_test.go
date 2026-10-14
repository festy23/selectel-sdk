package dns_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestCurrentDNSCatalogKeepsExactThirteenOperationsAndUnknownServiceInventory(t *testing.T) {
	c := catalog.Default()
	rows, e := c.List(catalog.Filter{Service: "dns"})
	if e != nil || len(rows) != 13 {
		t.Fatal(e, len(rows))
	}
	for _, r := range rows {
		if r.Implementation.Status != "implemented" || r.APIGeneration != "v2" || r.Auth.Scheme != "iam_project" || r.GoMethod == nil || r.ParameterSupport.CoveredSchemaRef == nil {
			t.Fatal(r.OperationID)
		}
		if r.Verification.Level != "none" && r.Verification.Level != "offline" {
			t.Fatal("live claim")
		}
	}
	r, e := c.Describe("dns.v2.zones.owner")
	if e != nil || len(r.WaitGoals) != 0 {
		t.Fatal("owner transfer is not propagation proof")
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil {
		t.Fatal("legacy/whole service inventory is unknown")
	}
}
