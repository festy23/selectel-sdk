package loadbalancers_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestLoadBalancerCatalogKeepsExactSubsetAndPartialVariants(t *testing.T) {
	c := catalog.Default()
	rows, e := c.List(catalog.Filter{Service: "loadbalancers"})
	if e != nil || len(rows) != 30 {
		t.Fatal(e, len(rows))
	}
	partial := map[string]bool{"loadbalancers.v2.loadbalancers.create": true, "loadbalancers.v2.listeners.create": true, "loadbalancers.v2.listeners.update": true, "loadbalancers.v2.pools.create": true, "loadbalancers.v2.pools.update": true}
	for _, row := range rows {
		want := "implemented"
		if partial[row.OperationID] {
			want = "partial"
		}
		if row.Implementation.Status != want || row.GoMethod == nil || (row.ParameterSupport.CoveredSchemaRef == nil || *row.ParameterSupport.CoveredSchemaRef == "") || row.Auth.Scheme != "iam_project" {
			t.Fatal(row.OperationID)
		}
		if row.Verification.Level != "none" && row.Verification.Level != "offline" {
			t.Fatal("live claim")
		}
		if partial[row.OperationID] && len(row.ParameterSupport.ExcludedVariants) == 0 {
			t.Fatal("partial without evidence")
		}
	}
	for _, id := range []string{"loadbalancers.v2.loadbalancers.failover", "loadbalancers.v2.members.batch_update"} {
		row, e := c.Describe(id)
		if e != nil || len(row.WaitGoals) != 0 {
			t.Fatal("invented causal completion", e)
		}
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil {
		t.Fatal("unknown inventory denominator")
	}
}
