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
		if r.Implementation.Status != "unimplemented" && ((r.Service != "registry_data" && r.Service != "billing" && r.Service != "secrets" && r.Service != "compute" && r.Service != "projects" && r.Service != "mks" && r.Service != "databases" && r.Service != "registry" && r.Service != "audit" && r.Service != "logs" && r.Service != "metrics") || (r.Verification.Level != "offline" && r.Verification.Level != "none")) {
			t.Fatal("unverified service overclaimed")
		}
	}
	all[0].Limitations[0] = "changed-by-caller"
	again, err := c.Describe(all[0].OperationID)
	if err != nil || again.Limitations[0] == "changed-by-caller" {
		t.Fatal("catalog is mutable")
	}
	s := c.Summary()
	if s.InventoryComplete || s.CoverageRatio != nil || s.Implemented < 53 || s.Partial < 25 {
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
	if json.Unmarshal(b, &doc) != nil || doc.SchemaVersion != 1 || len(doc.Operations) < 205 || doc.Summary.InventoryComplete || doc.Summary.CoverageRatio != nil {
		t.Fatal("serialized catalog loses contract or overclaims coverage")
	}
}

func TestRegistryOCIExplicitPartialBindings(t *testing.T) {
	own, e := catalog.Default().List(catalog.Filter{Service: "registry_data"})
	if e != nil || len(own) != 15 {
		t.Fatal(own, e)
	}
	ids := []string{"referrers.list", "version.get", "tags.list", "uploads.start", "uploads.status", "uploads.append", "uploads.finalize", "uploads.cancel_native", "blobs.get", "blobs.head", "blobs.delete", "manifests.get", "manifests.head", "manifests.replace", "manifests.delete"}
	c := catalog.Default()
	for _, id := range ids {
		cap, e := c.Describe("registry_data.oci_v1." + id)
		if e != nil || cap.Service != "registry_data" || cap.APIGeneration != "oci_v1" || cap.Implementation.Status != "partial" || cap.Auth.Scheme != types.Basic {
			t.Fatal(id, cap, e)
		}
	}
}

func TestRegistryNativeCancellationUsesPinnedDistributionSource(t *testing.T) {
	cap, e := catalog.Default().Describe("registry_data.oci_v1.uploads.cancel_native")
	if e != nil || cap.Source.URI != "https://github.com/distribution/distribution/blob/f7d69d28c2e302f31d1f6ce8e193da58407225a7/docs/content/spec/api.md" {
		t.Fatal(cap.Source, e)
	}
}
