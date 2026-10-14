package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/catalog"
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
	if e != nil || out.Data.Domain == nil || out.Data.Domain["used"].Value.String() != "900719925474099312345" || out.Data.Domain["used"].Start.UTC.Hour() != 0 || out.Data.Domain["paid"].Value.String() != "0" {
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

func TestResellSubnetCreateDecodesBothPublishedShapesAndPreservesSDKFields(t *testing.T) {
	for _, body := range []string{`{"subnet":{"id":42,"project_id":"p/one","vlan_id":4094,"vtep_ip_address":"192.0.2.4"}}`, `{"subnets":[{"id":42,"project_id":"p/one","vlan_id":4094,"vtep_ip_address":"192.0.2.4"}]}`} {
		c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
			if r.Method != "POST" || r.URL.EscapedPath() != "/vpc/resell/v2/subnets/projects/p%2Fone" {
				t.Fatal("wrong allocation target")
			}
			var request map[string][]map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&request) != nil || string(request["subnets"][0]["prefix_length"]) != "29" {
				t.Fatal("subnet body lost")
			}
			if _, ok := request["subnets"][0]["quantity"]; ok {
				t.Fatal("SDK omitted quantity default invented")
			}
			return inventoryReply(200, body), nil
		}, 0)
		prefix, _ := types.ParseInteger("29")
		out, e := c.Account().Resell().Subnets.Create(context.Background(), resell.SubnetCreateRequest{ProjectID: "p/one", Subnets: []resell.SubnetSpec{{Region: managementPtr("ru-2"), Type: managementPtr("ipv4"), PrefixLength: &prefix}}}, types.CallOptions{})
		if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Data) != 1 || out.Data[0].VlanID.String() != "4094" || *out.Data[0].VtepIPAddress != "192.0.2.4" || out.Data[0].Ref.Kind != "resell_subnet" || out.Receipt.ResourceRefs[0].ID != "42" {
			t.Fatal("published allocation variant lost", e)
		}
	}
}

func TestResellSubnetCreatePreservesAcceptedRefsWhenOtherFieldsMalformed(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"subnets":[{"id":42,"project_id":"p1","vlan_id":"invalid-integer"}]}`), nil
	}, 0)
	out, e := c.Account().Resell().Subnets.Create(context.Background(), resell.SubnetCreateRequest{ProjectID: "p1"}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "42" {
		t.Fatal("accepted allocation refs lost", e)
	}
}

func TestResellFloatingIPLifecycleUsesAllocationIdentityAndPublishedShapes(t *testing.T) {
	for _, shape := range []string{`{"floatingip":{"id":"f/one","project_id":"p1","floating_ip_address":"203.0.113.4"}}`, `{"floatingips":[{"id":"f/one","project_id":"p1","floating_ip_address":"203.0.113.4"}]}`} {
		c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
			switch r.Method {
			case "POST":
				if r.URL.Path != "/vpc/resell/v2/floatingips/projects/p1" {
					t.Fatal("wrong project")
				}
				return inventoryReply(200, shape), nil
			case "GET":
				if r.URL.EscapedPath() != "/vpc/resell/v2/floatingips/f%2Fone" {
					t.Fatal("wrong exact ref")
				}
				return inventoryReply(200, `{"floatingip":{"id":"f/one","project_id":"p1","fixed_ip_address":null,"port_id":null}}`), nil
			case "DELETE":
				return inventoryReply(204, ""), nil
			}
			t.Fatal("unexpected method")
			return nil, nil
		}, 0)
		service := c.Account().Resell().FloatingIPs
		out, e := service.Create(context.Background(), resell.FloatingIPCreateRequest{ProjectID: "p1", FloatingIPs: []resell.FloatingIPSpec{{Region: managementPtr("ru-2")}}}, types.CallOptions{})
		if e != nil || len(out.Data) != 1 || out.Data[0].Ref.Kind != "resell_floatingip" || out.Receipt.Outcome != types.Accepted {
			t.Fatal("create failed", e)
		}
		ref := *out.Data[0].Ref
		read, e := service.Get(context.Background(), ref, types.CallOptions{})
		if e != nil || read.Data.FixedIPAddress != nil || read.Data.PortID != nil || read.Data.Ref.ID != "f/one" {
			t.Fatal("get failed", e)
		}
		deleted, e := service.Delete(context.Background(), ref, types.CallOptions{})
		if e != nil || deleted.Receipt.Outcome != types.Accepted || deleted.Receipt.ResourceRefs[0] != ref {
			t.Fatal("delete receipt lost", e)
		}
	}
}

func TestResellDeprecatedLicensesRemainCallableWithReceipts(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case "POST":
			var body map[string][]map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["licenses"][0]["quantity"]) != "2" {
				t.Fatal("license request lost")
			}
			return inventoryReply(200, `{"licenses":[{"id":123,"project_id":"p1","type":"windows","region":"ru-2"}]}`), nil
		case "GET":
			if strings.HasSuffix(r.URL.Path, "/123") {
				return inventoryReply(200, `{"license":{"id":123,"project_id":"p1","port_id":"port1"}}`), nil
			}
			return inventoryReply(200, `{"licenses":[{"id":123,"project_id":"p1"}]}`), nil
		case "DELETE":
			return inventoryReply(204, ""), nil
		}
		return nil, io.ErrUnexpectedEOF
	}, 0)
	quantity, _ := types.ParseInteger("2")
	service := c.Account().Resell().Licenses
	created, e := service.Create(context.Background(), resell.LicenseCreateRequest{ProjectID: "p1", Licenses: []resell.LicenseSpec{{Quantity: &quantity, Region: managementPtr("ru-2"), Type: managementPtr("windows")}}}, types.CallOptions{})
	if e != nil || created.Receipt.Outcome != types.Accepted || created.Data[0].Ref.Kind != "resell_license" {
		t.Fatal("license create failed", e)
	}
	ref := *created.Data[0].Ref
	read, e := service.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || *read.Data.PortID != "port1" {
		t.Fatal("license get failed", e)
	}
	listed, e := service.Collect(context.Background(), resell.AllocationListOptions{Detailed: managementPtr(false)}, pagination.Options{})
	if e != nil || !listed.Complete || len(listed.Items) != 1 {
		t.Fatal("license list failed", e)
	}
	deleted, e := service.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal("license deletion failed", e)
	}
}

func TestResellLegacyKeypairLifecycleKeepsOwnerAndRegionVariants(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case "POST":
			return inventoryReply(200, `{"keypair":[{"name":"key/one","user_id":"user/one","region":"ru-2","public_key":"ssh-ed25519 synthetic"}]}`), nil
		case "GET":
			if r.URL.Query().Get("user_id") != "user/one" {
				t.Fatal("owner filter lost")
			}
			return inventoryReply(200, `{"keypairs":[{"name":"key/one","user_id":"user/one","regions":["ru-2"]}]}`), nil
		case "DELETE":
			if r.URL.EscapedPath() != "/vpc/resell/v2/keypairs/key%2Fone/users/user%2Fone" {
				t.Fatal("keypair owner target lost")
			}
			return inventoryReply(204, ""), nil
		}
		return nil, io.ErrUnexpectedEOF
	}, 0)
	service := c.Account().Resell().Keypairs
	created, e := service.Create(context.Background(), &resell.KeypairSpec{Name: managementPtr("key/one"), UserID: managementPtr("user/one"), PublicKey: managementPtr("ssh-ed25519 synthetic")}, types.CallOptions{})
	if e != nil || created.Receipt.Outcome != types.Accepted || len(created.Data) != 1 || *created.Data[0].Region != "ru-2" || created.Data[0].Ref.Parent.ID != "user/one" {
		t.Fatal("keypair create lost", e)
	}
	rows, e := service.Collect(context.Background(), resell.KeypairListOptions{UserID: managementPtr("user/one")}, pagination.Options{})
	if e != nil || !rows.Complete || len(rows.Items[0].Regions) != 1 {
		t.Fatal("keypair list lost", e)
	}
	deleted, e := service.Delete(context.Background(), *created.Data[0].Ref, types.CallOptions{})
	if e != nil || deleted.Receipt.ResourceRefs[0].Parent.ID != "user/one" || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal("keypair delete target lost", e)
	}
}

func TestResellTrafficKeepsOfficialSDKPrepaidAndTimezoneFreeTimestamp(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"traffic":{"domain":{"prepaid":{"start":"2018-04-01T00:00:00","stop":"2018-04-30T23:59:59","unit":"B","value":3000000000000}}}}`), nil
	}, 0)
	out, e := c.Account().Resell().Traffic(context.Background(), types.CallOptions{})
	if e != nil || out.Data.Domain["prepaid"].Start.Raw != "2018-04-01T00:00:00" || out.Data.Domain["prepaid"].Start.UTC != nil || out.Data.Domain["prepaid"].Value.String() != "3000000000000" {
		t.Fatal("SDK traffic type or timezone guessed", e)
	}
}

func TestResellCatalogBindsOfficialSDKSupplementAndAllRemoteMethods(t *testing.T) {
	c := catalog.Default()
	for _, id := range []string{"projects.v2.subnets.get", "projects.v2.subnets_projects.create", "projects.v2.floatingips.list", "projects.v2.floatingips_projects.create", "projects.v2.licenses_projects.create", "projects.v2.keypairs_users.delete", "projects.v2.traffic.list"} {
		op, e := c.Describe(id)
		if e != nil || op.Implementation.Status != "implemented" || op.ContractArtifactRevisions["contracts/resell-sdk-supplement.json"] == "" {
			t.Fatal("source-bound binding missing", id, e)
		}
	}
}

func TestResellProjectAllocationCannotCrossProjectAndListsAccountOnlyWhereRequired(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return inventoryReply(200, `{"subnets":[{"id":42,"project_id":"test-project"}]}`), nil
	}, 0)
	service := projectView(t, c).Resell()
	_, e := service.Subnets.Create(context.Background(), resell.SubnetCreateRequest{ProjectID: "other"}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 0 {
		t.Fatal("cross-project allocation reached provider", e)
	}
	created, e := service.Subnets.Create(context.Background(), resell.SubnetCreateRequest{ProjectID: "test-project"}, types.CallOptions{})
	if e != nil || created.Receipt.Scope.ProjectID != "test-project" || created.Data[0].Ref.Scope.ProjectID != "test-project" {
		t.Fatal("project scope lost", e)
	}
	_, e = service.FloatingIPs.Collect(context.Background(), resell.FloatingIPListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal("account-only floating list used project credentials", e)
	}
}

func TestResellCatalogAdvertisesProjectAuthOnlyForPublishedOperations(t *testing.T) {
	c := catalog.Default()
	op, e := c.Describe("projects.v2.subnets.get")
	if e != nil || len(op.Auth.Alternatives) != 1 || op.Auth.Alternatives[0] != types.IAMProject {
		t.Fatal("published project auth missing", e)
	}
	op, e = c.Describe("projects.v2.floatingips.list")
	if e != nil || len(op.Auth.Alternatives) != 0 {
		t.Fatal("account-only operation project auth invented", e)
	}
}

func TestResellAllocationEmptyResponseRemainsAcceptedWithoutInventedRefs(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) { return inventoryReply(200, `{"licenses":[]}`), nil }, 0)
	out, e := c.Account().Resell().Licenses.Create(context.Background(), resell.LicenseCreateRequest{ProjectID: "p1", Licenses: []resell.LicenseSpec{}}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || out.Data == nil || len(out.Data) != 0 || len(out.Receipt.ResourceRefs) != 0 {
		t.Fatal("empty accepted response invented a resource", e)
	}
}

func TestResellFloatingIPCollectionResumesAndPreservesStringFilter(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/vpc/resell/v2/floatingips" || r.URL.Query().Get("detailed") != "false" {
			t.Fatal("string detailed filter lost")
		}
		return inventoryReply(200, `{"floatingips":[{"id":"f1"},{"id":"f2"}]}`), nil
	}, 0)
	service := c.Account().Resell().FloatingIPs
	first, e := service.Collect(context.Background(), resell.FloatingIPListOptions{Detailed: managementPtr("false")}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || first.NextCursor == nil {
		t.Fatal("partial collection marked complete", e)
	}
	second, e := service.Collect(context.Background(), resell.FloatingIPListOptions{Detailed: managementPtr("false")}, pagination.Options{Cursor: first.NextCursor})
	if e != nil || !second.Complete || len(second.Items) != 1 || *second.Items[0].ID != "f2" || calls != 2 {
		t.Fatal("collection resume lost tail", e)
	}
}

func TestResellAllocationMutationsNeverReplayUnknownAndKeepRejection(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		code    types.Code
		outcome types.Outcome
	}{{0, "", types.UnknownOutcome, types.Unknown}, {400, `{"message":"synthetic-sensitive-marker"}`, types.ProviderError, types.Rejected}, {200, `{"floatingip":{"id":"f1","project_id":"other"}}`, types.ScopeMismatch, types.Accepted}, {200, `{"floatingip":{"id":"f1"},"floatingips":[{"id":"f2"}]}`, types.ProtocolError, types.Accepted}} {
		calls := 0
		c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
			calls++
			if tc.status == 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return inventoryReply(tc.status, tc.body), nil
		}, 0)
		out, e := c.Account().Resell().FloatingIPs.Create(context.Background(), resell.FloatingIPCreateRequest{ProjectID: "p1"}, types.CallOptions{})
		if types.ErrorCode(e) != tc.code || out.Receipt == nil || out.Receipt.Outcome != tc.outcome || calls != 1 {
			t.Fatal("mutation replay/receipt classification changed", e)
		}
		if strings.Contains(fmt.Sprintf("%+v", e), "synthetic-sensitive-marker") {
			t.Fatal("provider body exposed")
		}
	}
}

func TestResellAcceptedAllocationsScanAllIdentitiesBeforeReportingRowErrors(t *testing.T) {
	good := `{"id":42,"project_id":"target"}`
	for _, tc := range []struct {
		name, rows string
		code       types.Code
		ids        []string
	}{
		{"malformed before", `{"id":"bad"},` + good, types.ProtocolError, []string{"42"}},
		{"malformed after", good + `,{"id":"bad"}`, types.ProtocolError, []string{"42"}},
		{"foreign before same ID", `{"id":42,"project_id":"foreign"},` + good, types.ScopeMismatch, []string{"42"}},
		{"foreign after", good + `,{"id":99,"project_id":"foreign"}`, types.ScopeMismatch, []string{"42"}},
		{"duplicate before later good", `{"id":41,"project_id":"target"},{"id":41,"project_id":"target"},` + good, types.ProtocolError, []string{"41", "42"}},
		{"duplicate after earlier good", good + `,{"id":41,"project_id":"target"},{"id":41,"project_id":"target"}`, types.ProtocolError, []string{"42", "41"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				return inventoryReply(200, `{"subnets":[`+tc.rows+`]}`), nil
			}, 0)
			out, e := c.Account().Resell().Subnets.Create(context.Background(), resell.SubnetCreateRequest{ProjectID: "target"}, types.CallOptions{})
			if types.ErrorCode(e) != tc.code || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || calls != 1 || len(out.Receipt.ResourceRefs) != len(tc.ids) {
				t.Fatalf("accepted refs lost: receipt=%+v err=%v", out.Receipt, e)
			}
			for i, id := range tc.ids {
				if out.Receipt.ResourceRefs[i].ID != id || out.Receipt.ResourceRefs[i].Scope.AccountID != "test-account" {
					t.Fatalf("wrong trusted ref %+v", out.Receipt.ResourceRefs[i])
				}
			}
		})
	}
}

func TestResellAcceptedKeypairsScanAllIdentitiesAndKeepRegionVariants(t *testing.T) {
	good := `{"name":"key","user_id":"owner","region":"ru-2"}`
	for _, tc := range []struct {
		name, rows string
		code       types.Code
	}{
		{"malformed before", `{"name":false,"user_id":"owner"},` + good, types.ProtocolError},
		{"malformed after", good + `,{"name":"key","user_id":false}`, types.ProtocolError},
		{"foreign before", `{"name":"key","user_id":"foreign"},` + good, types.ScopeMismatch},
		{"foreign after", good + `,{"name":"foreign","user_id":"owner"}`, types.ScopeMismatch},
		{"repeated region identities", good + `,{"name":"key","user_id":"owner","region":"ru-3"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				return inventoryReply(200, `{"keypair":[`+tc.rows+`]}`), nil
			}, 0)
			out, e := c.Account().Resell().Keypairs.Create(context.Background(), &resell.KeypairSpec{Name: managementPtr("key"), UserID: managementPtr("owner"), PublicKey: managementPtr("synthetic-public-key")}, types.CallOptions{})
			if types.ErrorCode(e) != tc.code || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || calls != 1 || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "key" || out.Receipt.ResourceRefs[0].Parent.ID != "owner" {
				t.Fatalf("accepted keypair refs lost: receipt=%+v err=%v", out.Receipt, e)
			}
			if tc.code == "" && (len(out.Data) != 2 || *out.Data[1].Region != "ru-3") {
				t.Fatal("documented regional variants collapsed")
			}
		})
	}
}
