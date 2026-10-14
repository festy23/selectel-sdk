package images_test

import (
	"github.com/festy23/selectel-sdk/catalog"
	"testing"
)

func TestTenMetadataAndMembershipBindingsKeepPartialPatchExplicit(t *testing.T) {
	c := catalog.Default()
	rows, e := c.List(catalog.Filter{Service: "images"})
	if e != nil || len(rows) != 10 {
		t.Fatalf("%d %v", len(rows), e)
	}
	for _, id := range []string{"images.create", "images.get", "images.list", "images.delete", "members.create", "members.get", "members.list", "members.update", "members.delete", "images.update"} {
		cap, e := c.Describe("images.v2." + id)
		want := "implemented"
		if id == "images.update" {
			want = "partial"
		}
		if e != nil || cap.Implementation.Status != want || cap.APIGeneration != "v2" || len(cap.ScopeRequirements) != 4 {
			t.Fatalf("%+v %v", cap, e)
		}
	}
	if c.Summary().InventoryComplete || c.Summary().CoverageRatio != nil {
		t.Fatal("unknown source denominator overclaimed")
	}
}
