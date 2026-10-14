package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/resell"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

type resellTransport func(*http.Request) (*http.Response, error)

func (f resellTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResellAccountPreservesFalseAndRequiresAccountFields(t *testing.T) {
	for _, tc := range []struct {
		body string
		code types.Code
	}{{`{"account":{"enabled":false,"locked":true,"name":"synthetic","onboarding":false,"locks":[{"created_at":"2026-10-14","reason":"synthetic"}]}}`, ""}, {`{"account":{"enabled":false,"name":"synthetic","onboarding":false}}`, types.ProtocolError}} {
		c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.selectel.ru/vpc/resell/v2/accounts" {
				t.Fatal("wrong endpoint")
			}
			return inventoryReply(200, tc.body), nil
		}, 0)
		out, e := c.Account().Resell().Account(context.Background(), types.CallOptions{})
		if types.ErrorCode(e) != tc.code {
			t.Fatalf("error %v", e)
		}
		if e == nil && (out.Data.Enabled == nil || *out.Data.Enabled || out.Data.Onboarding == nil || *out.Data.Onboarding || len(out.Data.Locks) != 1) {
			t.Fatal("account fields lost")
		}
	}
}

func TestResellDNSCheckPreservesHostnameAsOnePathSegment(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/vpc/resell/v2/dnscheck/host%2Fone" {
			t.Fatal("hostname not escaped")
		}
		return inventoryReply(200, `{"result":{"content":"synthetic","delegated":false,"ns_servers":["ns.example"]}}`), nil
	}, 0)
	out, e := c.Account().Resell().DNSCheck(context.Background(), "host/one", types.CallOptions{})
	if e != nil || out.Data.Delegated == nil || *out.Data.Delegated || len(out.Data.NsServers) != 1 {
		t.Fatal("DNS info lost", e)
	}
}

func TestResellDNSCheckRejectsEmptyHostnameWithoutIOAndMapsNotFound(t *testing.T) {
	requests := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		return inventoryReply(400, `{"message":"synthetic-sensitive-marker"}`), nil
	}, 0)
	out, e := c.Account().Resell().DNSCheck(context.Background(), "", types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || requests != 0 || out.Receipt != nil {
		t.Fatal("invalid hostname reached provider", e)
	}
	_, e = c.Account().Resell().DNSCheck(context.Background(), "example.invalid", types.CallOptions{})
	if types.ErrorCode(e) != types.NotFound || requests != 1 {
		t.Fatal("documented not-found mapping lost", e)
	}
}

func TestResellTrafficPreservesBytePrecisionAndUTC(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/vpc/resell/v2/traffic" {
			t.Fatal("wrong traffic endpoint")
		}
		return inventoryReply(200, `{"traffic":{"domain":{"used":{"value":900719925474099312345,"unit":"bytes","start":"2026-10-14T03:00:00+03:00","stop":"2026-10-15T03:00:00+03:00"},"paid":{"value":0}}}}`), nil
	}, 0)
	out, e := c.Account().Resell().Traffic(context.Background(), types.CallOptions{})
	if e != nil || out.Data.Domain == nil || out.Data.Domain.Used.Value.String() != "900719925474099312345" || out.Data.Domain.Used.Start.Hour() != 0 || out.Data.Domain.Paid.Value.String() != "0" {
		t.Fatal("traffic values lost", e)
	}
}

func TestResellTrafficReportValidatesDatesAndProtectsCSV(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/vpc/resell/v2/traffic/report" || r.URL.Query().Get("start_date") != "2026-10-01" || r.URL.Query().Get("end_date") != "2026-10-14" {
			t.Fatal("report query lost")
		}
		return inventoryReply(200, "project,traffic\nsynthetic-secret-marker,123\n"), nil
	}, 0)
	service := c.Account().Resell()
	_, e := service.TrafficReport(context.Background(), resell.TrafficReportRequest{StartDate: "2026-02-30", EndDate: "2026-10-14"}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal("invalid date reached provider", e)
	}
	out, e := service.TrafficReport(context.Background(), resell.TrafficReportRequest{StartDate: "2026-10-01", EndDate: "2026-10-14"}, types.CallOptions{})
	if e != nil || string(out.Data.Reveal()) != "project,traffic\nsynthetic-secret-marker,123\n" || strings.Contains(fmt.Sprint(out.Data), "synthetic-secret-marker") {
		t.Fatal("report bytes or protection lost", e)
	}
}

func TestResellSubnetsAreAllocationRefsWithBoundedCollection(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/vpc/resell/v2/subnets" || r.URL.Query().Get("detailed") != "false" {
			t.Fatal("subnet query lost")
		}
		return inventoryReply(200, `{"subnets":[{"id":900719925474099312345,"project_id":"p1","subnet_id":"neutron-1","cidr":"203.0.113.0/29"},{"id":2,"project_id":"p2"}]}`), nil
	}, 0)
	out, e := c.Account().Resell().Subnets.Collect(context.Background(), resell.AllocationListOptions{Detailed: managementPtr(false)}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].ID.String() != "900719925474099312345" || out.Items[0].Ref.Kind != "resell_subnet" || out.Items[0].Ref.ID == *out.Items[0].SubnetID {
		t.Fatal("allocation identity or incompleteness lost", e)
	}
}

func TestResellSubnetExactGetRejectsForeignIdentity(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/vpc/resell/v2/subnets/42" {
			t.Fatal("wrong subnet ref")
		}
		return inventoryReply(200, `{"subnet":{"id":43,"subnet_id":"neutron-1"}}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "projects", APIGeneration: "v2", Kind: "resell_subnet", ID: "42", EndpointID: "selectel-resell", EndpointURL: "https://api.selectel.ru/vpc/resell"}
	_, e := c.Account().Resell().Subnets.Get(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal("foreign allocation adopted", e)
	}
	ref.Kind = "subnet"
	_, e = c.Account().Resell().Subnets.Get(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 1 {
		t.Fatal("Neutron ref reached Resell", e)
	}
}

func TestResellSubnetDeletePreservesUnknownReceiptWithoutReplay(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/vpc/resell/v2/subnets/42" {
			t.Fatal("wrong deletion")
		}
		return nil, io.ErrUnexpectedEOF
	}, 0)
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "projects", APIGeneration: "v2", Kind: "resell_subnet", ID: "42", EndpointID: "selectel-resell", EndpointURL: "https://api.selectel.ru/vpc/resell"}
	out, e := c.Account().Resell().Subnets.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "42" {
		t.Fatal("unknown delete replayed or lost receipt", e)
	}
}

func TestResellPublishedSchemaProtectsArbitraryPayload(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/vpc/resell/v2/swagger/doc.json" {
			t.Fatal("wrong schema path")
		}
		return inventoryReply(200, `{"swagger":"2.0","description":"synthetic-secret-marker"}`), nil
	}, 0)
	out, e := c.Account().Resell().PublishedSchema(context.Background(), types.CallOptions{})
	raw, err := json.Marshal(out.Data)
	if e != nil || err != nil || strings.Contains(string(raw), "synthetic-secret-marker") || !strings.Contains(string(out.Data.Reveal()), "synthetic-secret-marker") {
		t.Fatal("schema protection lost", e)
	}
}
