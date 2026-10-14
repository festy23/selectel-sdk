package publicips_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestPublicIPCatalogKeepsAPIKindsAndSourceLimitations(t *testing.T) {
	c := catalog.Default()
	ids := []string{"projects.v2.floatingips.get", "projects.v2.floatingips.list", "projects.v2.floatingips.delete", "projects.v2.floatingips_projects.create", "network.v2.floatingips.get", "network.v2.floatingips.list", "network.v2.floatingips.create", "network.v2.floatingips.update", "network.v2.floatingips.delete", "public_network.v1.direct_ports.get", "public_network.v1.direct_ports.list", "public_network.v1.direct_ports.create", "public_network.v1.direct_ports.update", "public_network.v1.direct_ports.delete", "public_network.v1.quotas.get"}
	for _, id := range ids {
		v, e := c.Describe(id)
		expected := "implemented"
		if e != nil || v.Implementation.Status != expected {
			t.Fatalf("binding %s %+v %v", id, v, e)
		}
	}
	if c.Summary().InventoryComplete || c.Summary().CoverageRatio != nil {
		t.Fatal("unknown denominator advertised")
	}
}
