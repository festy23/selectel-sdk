package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/types"
)

func TestOfflineCatalogIsImmutableAndDoesNotOverclaim(t *testing.T) {
	c := catalog.Default()
	all, err := c.List(catalog.Filter{})
	if err != nil || len(all) < 100 {
		t.Fatal("source inventory missing")
	}
	for i, r := range all {
		if i > 0 && all[i-1].OperationID >= r.OperationID {
			t.Fatal("unordered or duplicate ids")
		}
		if r.Implementation.Status != "unimplemented" && ((r.Service != "billing" && r.Service != "secrets" && r.Service != "compute" && r.Service != "projects" && r.Service != "mks" && r.Service != "databases" && r.Service != "registry" && r.Service != "audit" && r.Service != "logs" && r.Service != "metrics" && r.Service != "dns") || (r.Verification.Level != "offline" && r.Verification.Level != "none")) {
			t.Fatal("unverified service overclaimed")
		}
	}
	all[0].Limitations[0] = "changed-by-caller"
	again, err := c.Describe(all[0].OperationID)
	if err != nil || again.Limitations[0] == "changed-by-caller" {
		t.Fatal("catalog is mutable")
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil || s.Implemented != 68 || s.Partial != 15 {
		t.Fatal("unknown denominator advertised as covered")
	}
	if _, err := c.List(catalog.Filter{Verification: "unknown"}); types.ErrorCode(err) != types.InvalidArgument {
		t.Fatal("invalid filter accepted")
	}
}

func TestSourceArtifactsAreAvailableAndCopied(t *testing.T) {
	c := catalog.Default()
	r, err := c.Describe("audit.v1.logs.query")
	if err != nil {
		t.Fatal(err)
	}
	if r.Auth.Scheme != types.IAMAccount || r.Effect != "read" {
		t.Fatal("audit scope or semantics wrong")
	}
	b, err := c.Artifact(*r.InputSchemaRef)
	if err != nil || !json.Valid(b) {
		t.Fatal("source contract absent")
	}
	b[0] = 'X'
	again, err := c.Artifact(*r.InputSchemaRef)
	if err != nil || !json.Valid(again) {
		t.Fatal("artifact mutated")
	}
}

func TestSerializedCatalogHasVersionAndUncertainCoverage(t *testing.T) {
	b, err := json.Marshal(catalog.Default())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SchemaVersion int                  `json:"schema_version"`
		Operations    []catalog.Capability `json:"operations"`
		Summary       struct {
			InventoryComplete bool     `json:"inventory_complete"`
			CoverageRatio     *float64 `json:"coverage_ratio"`
		} `json:"summary"`
	}
	if json.Unmarshal(b, &doc) != nil || doc.SchemaVersion != 1 || len(doc.Operations) != 210 || doc.Summary.InventoryComplete || doc.Summary.CoverageRatio != nil {
		t.Fatal("serialized catalog loses contract or overclaims coverage")
	}
}
