package selectelsdk_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestHeatDiscoveryPinsComponentEvidenceWithoutInventingOperations(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	var heat *catalog.Service
	for _, s := range c.Catalog().Services() {
		if s.ID == "heat" {
			copy := s
			heat = &copy
		}
	}
	if heat == nil || heat.Availability != "discovery_pending" || heat.InventoryComplete || heat.Discovery == nil {
		t.Fatalf("Heat discovery absent or overclaimed: %+v", heat)
	}
	evidence := heat.Discovery
	if len(evidence.Revision) != 64 || evidence.InventoryCount != nil || len(evidence.Sources) == 0 || len(evidence.Versions) != 1 || evidence.Versions[0].Release != "Zed" || evidence.Versions[0].APIVersion != "1.0" || len(evidence.FollowupIssues) == 0 || len(evidence.MissingContracts) == 0 {
		t.Fatalf("incomplete discovery evidence: %+v", evidence)
	}
	rows, err := c.Catalog().List(catalog.Filter{Service: "heat"})
	if err != nil || len(rows) != 0 {
		t.Fatal("upstream operations invented as Selectel API coverage")
	}
	raw, err := c.Catalog().Artifact(evidence.EvidenceRef)
	if err != nil || len(raw) == 0 {
		t.Fatal("pinned local source evidence absent")
	}
	summary := c.Catalog().Summary()
	if summary.InventoryComplete || summary.CoverageRatio != nil || summary.Documented != 191 || summary.Implemented != 53 || summary.Partial != 11 || calls != 0 {
		t.Fatalf("discovery overclaimed/probed: %+v calls=%d", summary, calls)
	}
}

func TestDiscoveryFamiliesKeepUnknownScopeAndInventoryDistinct(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]struct{ release, api, max string }{"heat": {"Zed", "1.0", ""}, "placement": {"Zed", "1.0", "1.39"}, "gnocchi": {"4.5", "1.0", ""}, "kms": {}, "hive_metastore": {"4.2.0", "", ""}}
	found := map[string]bool{}
	for _, service := range c.Catalog().Services() {
		want, ok := expected[service.ID]
		if !ok {
			continue
		}
		found[service.ID] = true
		evidence := service.Discovery
		if service.Availability != "discovery_pending" || service.InventoryComplete || evidence == nil || evidence.InventoryCount != nil || len(evidence.Sources) == 0 || len(evidence.FollowupIssues) != 1 {
			t.Fatalf("family %s overclaimed: %+v", service.ID, service)
		}
		if service.ID == "kms" {
			if len(evidence.Versions) != 0 {
				t.Fatal("KMS protocol/version invented")
			}
		} else {
			if len(evidence.Versions) != 1 || evidence.Versions[0].Release != want.release || evidence.Versions[0].APIVersion != want.api || evidence.Versions[0].MaxMicroversion != want.max {
				t.Fatalf("version boundary %s: %+v", service.ID, evidence.Versions)
			}
		}
		rows, e := c.Catalog().List(catalog.Filter{Service: service.ID})
		if e != nil || len(rows) != 0 {
			t.Fatalf("%s fabricated operation inventory", service.ID)
		}
	}
	if len(found) != len(expected) {
		t.Fatalf("missing families: %+v", found)
	}
}

func TestDiscoveryEvidenceIsImmutableAndContentBound(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range c.Catalog().Services() {
		if service.ID != "heat" {
			continue
		}
		d := service.Discovery
		if d == nil {
			t.Fatal("missing evidence")
		}
		raw, e := c.Catalog().Artifact(d.EvidenceRef)
		if e != nil {
			t.Fatal(e)
		}
		hash := sha256.Sum256(raw)
		if d.Revision != hex.EncodeToString(hash[:]) {
			t.Fatal("discovery evidence revision is not content-bound")
		}
		d.Sources[0].Facts[0] = "caller mutation"
		d.Sources[0].Revision = "caller mutation"
		d.Versions[0].Release = "caller mutation"
		d.FollowupIssues[0] = "caller mutation"
		d.MissingContracts[0] = "caller mutation"
		raw[0] = 'X'
	}
	for _, service := range c.Catalog().Services() {
		if service.ID != "heat" {
			continue
		}
		raw, e := c.Catalog().Artifact(service.Discovery.EvidenceRef)
		if e != nil || !json.Valid(raw) {
			t.Fatal("mutable artifact")
		}
		d := service.Discovery
		if d.Sources[0].Facts[0] == "caller mutation" || d.Sources[0].Revision == "caller mutation" || d.Versions[0].Release != "Zed" || d.FollowupIssues[0] == "caller mutation" || d.MissingContracts[0] == "caller mutation" {
			t.Fatal("mutable evidence slices")
		}
	}
}
