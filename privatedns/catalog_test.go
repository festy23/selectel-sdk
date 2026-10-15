package privatedns_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestSourceBoundPrivateDNSAndPTRCoverage(t *testing.T) {
	c := catalog.Default()
	for service, n := range map[string]int{"private_dns": 13, "ip_addresses": 5} {
		ops, e := c.List(catalog.Filter{Service: service})
		if e != nil || len(ops) != n {
			t.Fatal(e, len(ops))
		}
		for _, op := range ops {
			want := "implemented"
			if op.OperationID == "ip_addresses.v1.ptrs.list" {
				want = "partial"
			}
			if op.Implementation.Status != want {
				t.Fatalf("%s %s", op.OperationID, op.Implementation.Status)
			}
		}
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil {
		t.Fatal("invented denominator")
	}
}
