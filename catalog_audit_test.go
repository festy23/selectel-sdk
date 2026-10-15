package selectelsdk_test

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func TestCatalogAuditSeparatesPinnedSourceSubsetFromWholeInventory(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "audit-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	audit := c.Catalog().Audit()
	if audit.InventoryComplete || audit.CoverageRatio != nil || len(audit.Sources) != 11 {
		t.Fatalf("invented inventory: %+v", audit)
	}
	found := false
	for _, src := range audit.Sources {
		if src.URI != "https://docs.selectel.ru/api/balance-statistics.json" {
			continue
		}
		found = true
		if src.Documented != 4 || src.Implemented != 2 || src.Partial != 0 || len(src.UnimplementedIDs) != 2 || len(src.Revision) != 64 {
			t.Fatalf("source reconciliation: %+v", src)
		}
		if src.UnimplementedIDs[0] != "billing.v1.cloud_billing_statistic_objects.list" || src.UnimplementedIDs[1] != "billing.v1.cloud_billing_statistic_projects.list" {
			t.Fatalf("missing canonical IDs: %+v", src.UnimplementedIDs)
		}
	}
	if !found || calls != 0 {
		t.Fatalf("source absent or audit performed IO calls=%d", calls)
	}
}

func TestCatalogAuditRetainsPartialVariantsAndIndependentCopies(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "audit-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	report := c.Catalog().Audit()
	found := false
	for _, src := range report.Sources {
		for _, gap := range src.PartialBindings {
			if gap.OperationID != "compute.v2_1.servers.create" {
				continue
			}
			found = true
			if gap.GoMethod != "compute.Servers.Create" || len(gap.ExcludedVariants) == 0 || gap.CoveredSchemaRef == "" {
				t.Fatalf("partial qualification lost: %+v", gap)
			}
			gap.ExcludedVariants[0] = "caller-mutated"
		}
	}
	if !found {
		t.Fatal("partial binding hidden")
	}
	for _, src := range c.Catalog().Audit().Sources {
		for _, gap := range src.PartialBindings {
			for _, variant := range gap.ExcludedVariants {
				if variant == "caller-mutated" {
					t.Fatal("mutable audit evidence")
				}
			}
		}
	}
}

func TestCatalogAuditDoesNotDropDiscoveryPendingDenominator(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "audit-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range c.Catalog().Audit().IncompleteFamilies {
		if family == "kms" {
			found = true
		}
	}
	if !found {
		t.Fatal("discovery-pending KMS omitted from unknown denominator")
	}
}

func TestCatalogAuditChecksBoundMethodDeclarationAndSchema(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "audit-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range c.Catalog().Audit().BindingChecks {
		if check.OperationID != "compute.v2_1.servers.create" {
			continue
		}
		found = true
		if check.GoMethod != "compute.Servers.Create" || check.MethodFile != "compute/servers.go" || len(check.MethodRevision) != 64 || len(check.Issues) != 0 {
			t.Fatalf("binding declaration/schema unresolved: %+v", check)
		}
	}
	if !found {
		t.Fatal("bound operation omitted from structural audit")
	}
}

func TestCatalogAuditChecksEveryCallableMappingWithoutPromotingCoverage(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "audit-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	report := c.Catalog().Audit()
	if len(report.OrphanBindings) != 0 || len(report.ReconciliationIssues) != 0 {
		t.Fatalf("shipped binding/source join is unresolved: %+v", report)
	}
	if len(report.BindingChecks) != 64 {
		t.Fatalf("callable binding audit lost records: %d", len(report.BindingChecks))
	}
	for _, check := range report.BindingChecks {
		if len(check.Issues) != 0 {
			t.Errorf("%s: %v", check.OperationID, check.Issues)
		}
	}
	if report.InventoryComplete || report.CoverageRatio != nil {
		t.Fatal("structural checks promoted unknown coverage")
	}
}
