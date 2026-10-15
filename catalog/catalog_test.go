package catalog_test

import (
	"encoding/json"
	"strings"
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
		if r.Implementation.Status != "unimplemented" && ((r.Service != "billing" && r.Service != "secrets" && r.Service != "compute" && r.Service != "projects" && r.Service != "mks" && r.Service != "databases" && r.Service != "registry" && r.Service != "audit" && r.Service != "logs" && r.Service != "metrics") || (r.Verification.Level != "offline" && r.Verification.Level != "none")) {
			t.Fatal("unverified service overclaimed")
		}
	}
	all[0].Limitations[0] = "changed-by-caller"
	again, err := c.Describe(all[0].OperationID)
	if err != nil || again.Limitations[0] == "changed-by-caller" {
		t.Fatal("catalog is mutable")
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil || s.Implemented < 53 || s.Partial < 11 {
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
	if json.Unmarshal(b, &doc) != nil || doc.SchemaVersion != 1 || len(doc.Operations) < 191 || doc.Summary.InventoryComplete || doc.Summary.CoverageRatio != nil {
		t.Fatal("serialized catalog loses contract or overclaims coverage")
	}
}

func TestRegistryTokenBindingsAreCompleteAndGenerationSpecific(t *testing.T) {
	c := catalog.Default()
	ids := []string{"registry.v1.token.create", "registry.v1.token.get", "registry.v1.token.delete", "registry.v1.token_refresh.create", "registry.v2.tokens.create", "registry.v2.tokens.get", "registry.v2.tokens.list", "registry.v2.tokens.update", "registry.v2.tokens_refresh.create", "registry.v2.tokens_regenerate.create", "registry.v2.tokens_revoke.create", "registry.v2.tokens.delete"}
	for _, id := range ids {
		v, e := c.Describe(id)
		expected := "v2"
		if strings.HasPrefix(id, "registry.v1.") {
			expected = "v1"
		}
		if e != nil || v.APIGeneration != expected || v.Implementation.Status != "implemented" || v.Service != "registry" || v.RequestBindingRef == nil || v.ResponseBindingRef == nil || v.ErrorBindingRef == nil {
			t.Fatal(id, v, e)
		}
	}
	for _, id := range []string{"registry.v1.registries.create", "registry.v1.registries.get", "registry.v1.registries.delete", "registry.v1.registries.list", "registry.v2.registries.list"} {
		v, e := c.Describe(id)
		if e != nil || v.Implementation.Status != "implemented" {
			t.Fatal(id, v, e)
		}
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil {
		t.Fatal(s)
	}
}
