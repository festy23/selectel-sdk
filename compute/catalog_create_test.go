package compute_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/compute"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func catalogClient(t *testing.T, wire transport) *compute.Client {
	t.Helper()
	scope := types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.IAMToken, Binding: scope, Audience: "selectel", Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	c, e := sdk.New(sdk.Config{AccountID: "account", Credentials: source, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "compute", APIGeneration: "v2_1", Scope: scope}: {ID: "compute-endpoint", URL: "https://nova.test/v2.1/project", Scope: scope, APIGeneration: "v2_1", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"project","domain":{"name":"account"}}}}`), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic-token" || r.URL.Host != "nova.test" {
			t.Fatal("auth/endpoint lost")
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("project")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("region", "pool")
	if e != nil {
		t.Fatal(e)
	}
	return r.Compute()
}
func TestFlavorCatalogPreservesNumbersAndBounds(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2.1/project/flavors/detail" || r.URL.Query().Get("minRam") != "4096" {
			t.Fatalf("flavor request %s", r.URL)
		}
		return response(200, `{"flavors":[{"id":"f1","name":"large","ram":9007199254740993,"vcpus":4,"disk":0,"swap":"","rxtx_factor":0.1234567890123456789,"os-flavor-access:is_public":false},{"id":"f2","name":"small"}]}`), nil
	})
	got, e := c.Flavors.Collect(context.Background(), compute.FlavorFilters{MinRAM: 4096}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || got.Complete || len(got.Items) != 1 || got.Items[0].RAM.String() != "9007199254740993" || got.Items[0].RxTxFactor.String() != "0.1234567890123456789" || got.Items[0].Swap.String() != "0" || got.Items[0].Ref == nil || got.Items[0].Ref.Kind != "flavor" {
		t.Fatalf("flavors %+v %v", got, e)
	}
}

var _ = json.Marshal
var _ = strings.Contains

func TestCreateSelectel293NativeFieldsAndProtectedPayload(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.Header.Get("OpenStack-API-Version") != "compute 2.93" || r.Header.Get("X-OpenStack-Nova-API-Version") != "2.93" {
			t.Fatalf("create version %s %v", r.Method, r.Header)
		}
		var body map[string]map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		s := body["server"]
		for k, want := range map[string]string{"networks": `"auto"`, "hostname": `"vm-host"`, "hypervisor_hostname": `"node.example"`, "tags": `["agent"]`, "user_data": `"cHJpdmF0ZS11c2VyLWRhdGE="`} {
			if string(s[k]) != want {
				t.Fatalf("%s=%s expected%s", k, s[k], want)
			}
		}
		return response(202, `{"server":{"id":"created","adminPass":"private-result"}}`), nil
	})
	user := types.Protect([]byte("private-user-data"))
	r := compute.CreateRequest{Microversion: "2.93", Options: servers.CreateOpts{Name: "vm", FlavorRef: "flavor", ImageRef: "image", Networks: "auto", Hostname: "vm-host", HypervisorHostname: "node.example", Tags: []string{"agent"}}, UserData: &user}
	got, e := c.Servers.Create(context.Background(), r)
	if e != nil || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || got.Data.Ref.ID != "created" {
		t.Fatalf("create %+v %v", got, e)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "private-result") {
		t.Fatal("secret leaked")
	}
}

func TestCreateLegacyPersonalityAndExplicitExtensionFields(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		var server map[string]json.RawMessage
		_ = json.Unmarshal(body["server"], &server)
		if string(server["personality"]) != `[{"contents":"cHJpdmF0ZS1maWxl","path":"/etc/config"}]` || string(server["description"]) != "null" || string(server["key_name"]) != `"ssh-key"` || !strings.Contains(string(body["os:scheduler_hints"]), "group") {
			t.Fatalf("extensions %s %s", body["server"], body["os:scheduler_hints"])
		}
		return response(202, `{"server":{"id":"created"}}`), nil
	})
	key := "ssh-key"
	r := request()
	r.Microversion = "2.56"
	r.Options.Networks = []servers.Network{{UUID: "00000000-0000-4000-8000-000000000002"}}
	r.KeyName = &key
	r.Description = types.Null[string]()
	path := "/etc/config"
	content := types.Protect([]byte("private-file"))
	r.Personality = []compute.PersonalityFile{{Path: &path, Contents: &content}}
	r.SchedulerHints = &servers.SchedulerHintOpts{Group: "00000000-0000-4000-8000-000000000001"}
	got, e := c.Servers.Create(context.Background(), r)
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("create %+v %v", got, e)
	}
}

func TestCreateReservationRetainsExactReceiptAndNeverReplays(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(fmt.Sprint(loss), func(t *testing.T) {
			calls := 0
			c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]map[string]json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&body)
				if string(body["server"]["return_reservation_id"]) != "true" || string(body["server"]["min_count"]) != "2" || string(body["server"]["max_count"]) != "3" {
					t.Fatal("multi-create fields lost")
				}
				if loss {
					return nil, io.ErrUnexpectedEOF
				}
				return response(202, `{"reservation_id":"r-exact"}`), nil
			})
			r := request()
			yes := true
			r.ReturnReservationID = &yes
			r.Options.Min = 2
			r.Options.Max = 3
			got, e := c.Servers.CreateWithResult(context.Background(), r)
			if calls != 1 || got.Receipt == nil {
				t.Fatalf("receipt %+v %v", got, e)
			}
			if loss {
				if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown || len(got.Receipt.ResourceRefs) != 0 {
					t.Fatalf("unknown %+v %v", got, e)
				}
			} else {
				if e != nil || got.Data.ReservationID == nil || *got.Data.ReservationID != "r-exact" || got.Data.Server != nil || got.Receipt.Recovery != "unavailable" || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].Kind != "server_reservation" || got.Receipt.ResourceRefs[0].ID != "r-exact" {
					t.Fatalf("reservation %+v %v", got, e)
				}
			}
		})
	}
}

func TestFlavorGetAndExtraSpecsUseExactRef(t *testing.T) {
	calls := 0
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "os-extra_specs") {
			return response(200, `{"extra_specs":{"hw:cpu_policy":"dedicated","quota:cpu_period":"100000"}}`), nil
		}
		return response(200, `{"flavor":{"id":"f1","disk":0,"description":null,"extra_specs":{"hw:cpu_policy":"dedicated"}}}`), nil
	})
	ref := serverRef()
	ref.ID = "f1"
	ref.Kind = "flavor"
	got, e := c.Flavors.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref.ID != "f1" || got.Data.Disk.String() != "0" || got.Data.Description == nil {
		t.Fatalf("flavor %+v %v", got, e)
	}
	specs, e := c.Flavors.ExtraSpecs(context.Background(), ref, types.CallOptions{})
	if e != nil || specs.Data.ExtraSpecs["hw:cpu_policy"] != "dedicated" {
		t.Fatalf("specs %+v %v", specs, e)
	}
	ref.Scope.ProjectID = "foreign"
	if _, e := c.Flavors.Get(context.Background(), ref, types.CallOptions{}); types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatalf("foreign ref %v %d", e, calls)
	}
}

func TestCreateTypedBlockDevicesAndNullableNetworkPort(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		s := body["server"]
		if string(s["networks"]) != `[{"port":null,"uuid":"00000000-0000-4000-8000-000000000002"}]` || !strings.Contains(string(s["block_device_mapping_v2"]), `"volume_type":"fast"`) || !strings.Contains(string(s["block_device_mapping_v2"]), `"boot_index":null`) {
			t.Fatalf("typed create %s %s", s["networks"], s["block_device_mapping_v2"])
		}
		return response(202, `{"server":{"id":"created"}}`), nil
	})
	network := "00000000-0000-4000-8000-000000000002"
	source, destination, volumeType, image := "image", "volume", "fast", "image-id"
	r := request()
	r.Microversion = "2.93"
	r.NetworkAttachments = []compute.NetworkAttachment{{UUID: &network, Port: types.Null[string]()}}
	r.BlockDevices = []compute.BlockDeviceMapping{{SourceType: &source, DestinationType: &destination, ImageID: &image, VolumeType: &volumeType, BootIndex: types.Null[types.Integer]()}}
	got, e := c.Servers.Create(context.Background(), r)
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("typed create %+v %v", got, e)
	}
}

func TestKeypairsCreateListGetDeleteProtectsPrivateKey(t *testing.T) {
	calls := 0
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("user_id") != "owner" && r.Method != "POST" {
			t.Fatal("owner query lost")
		}
		switch r.Method {
		case "POST":
			var body map[string]map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["keypair"]["user_id"] != "owner" || body["keypair"]["name"] != "ssh" {
				t.Fatal("keypair body")
			}
			return response(201, `{"keypair":{"name":"ssh","user_id":"owner","type":"ssh","public_key":"ssh-public","private_key":"private-ssh-key"}}`), nil
		case "DELETE":
			return response(204, ""), nil
		default:
			if strings.HasSuffix(r.URL.Path, "/os-keypairs") {
				if r.URL.Query().Get("marker") != "" {
					return response(200, `{"keypairs":[]}`), nil
				}
				return response(200, `{"keypairs":[{"keypair":{"name":"ssh","user_id":"owner","fingerprint":"fp"}}]}`), nil
			}
			return response(200, `{"keypair":{"name":"ssh","user_id":"owner","id":9007199254740993,"deleted":false,"deleted_at":null,"updated_at":null,"created_at":"2026-10-14T12:00:00.000000"}}`), nil
		}
	})
	got, e := c.Keypairs.Create(context.Background(), compute.KeypairCreateRequest{Name: "ssh", UserID: "owner"}, types.CallOptions{})
	if e != nil || got.Data.PrivateKey == nil || string(got.Data.PrivateKey.Reveal()) != "private-ssh-key" || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("keypair %+v %v", got, e)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "private-ssh-key") || strings.Contains(fmt.Sprintf("%+v %#v", got, got), "private-ssh-key") {
		t.Fatal("private key leaked")
	}
	list, e := c.Keypairs.Collect(context.Background(), compute.KeypairFilters{UserID: "owner"}, pagination.Options{})
	if e != nil || !list.Complete || len(list.Items) != 1 || list.Items[0].Ref.Parent.ID != "owner" {
		t.Fatalf("keys %+v %v", list, e)
	}
	detail, e := c.Keypairs.Get(context.Background(), *list.Items[0].Ref, types.CallOptions{})
	if e != nil || detail.Data.ID.String() != "9007199254740993" || detail.Data.CreatedAt.String() != "2026-10-14T12:00:00.000000" || detail.Data.DeletedAt == nil {
		t.Fatalf("key detail %+v %v", detail, e)
	}
	deleted, e := c.Keypairs.Delete(context.Background(), *detail.Data.Ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 5 {
		t.Fatalf("delete %+v %v", deleted, e)
	}
}

func TestCreateLegacyImplicitNetworkAndMultipleFirstServer(t *testing.T) {
	calls := 0
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, present := body["server"]["networks"]; present {
			t.Fatal("implicit legacy network field invented")
		}
		if string(body["server"]["max_count"]) != "3" {
			t.Fatal("multiple count lost")
		}
		return response(202, `{"server":{"id":"first-server"}}`), nil
	})
	r := request()
	r.Options.Networks = nil
	r.Options.Min = 2
	r.Options.Max = 3
	got, e := c.Servers.Create(context.Background(), r)
	if e != nil || got.Data.Ref.ID != "first-server" || calls != 1 {
		t.Fatalf("multiple %+v %v", got, e)
	}
	wait, e := c.Servers.CreateAndWait(context.Background(), r)
	if types.ErrorCode(e) != types.InvalidArgument || wait.Create.Receipt == nil || wait.Create.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatalf("batch wait %+v %v", wait, e)
	}
}

func TestKeypairsCurrentAndNamedRefsCannotCross(t *testing.T) {
	calls := 0
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Has("user_id") {
			t.Fatal("current route got named-owner query")
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		if strings.HasSuffix(r.URL.Path, "/os-keypairs") {
			return response(200, `{"keypairs":[{"keypair":{"name":"current-key"}}]}`), nil
		}
		return response(200, `{"keypair":{"name":"current-key","user_id":"actual-owner"}}`), nil
	})
	page, e := c.Keypairs.Current.ListPage(context.Background(), pagination.Options{})
	if e != nil || len(page.Items) != 1 || page.Items[0].Ref.Parent.Kind != "keypair_current_user" {
		t.Fatalf("current keys %+v %v", page, e)
	}
	relative := *page.Items[0].Ref
	if _, e := c.Keypairs.Get(context.Background(), relative, types.CallOptions{}); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatalf("named accepted relative %v", e)
	}
	named := relative
	parent := *relative.Parent
	parent.Kind = "keypair_user"
	parent.ID = "owner"
	named.Parent = &parent
	if _, e := c.Keypairs.Current.Delete(context.Background(), named, types.CallOptions{}); types.ErrorCode(e) != types.InvalidArgument || calls != 1 {
		t.Fatalf("current accepted named %v calls%d", e, calls)
	}
	got, e := c.Keypairs.Current.Get(context.Background(), relative, types.CallOptions{})
	if e != nil || got.Data.Ref.Parent.Kind != "keypair_user" || got.Data.Ref.Parent.ID != "actual-owner" {
		t.Fatalf("returned owner %+v %v", got, e)
	}
	deleted, e := c.Keypairs.Current.Delete(context.Background(), relative, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatalf("current delete %+v %v", deleted, e)
	}
}
func TestKeypairCreationUnknownAndMalformedKeepsExactTarget(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(fmt.Sprint(loss), func(t *testing.T) {
			calls := 0
			c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if loss {
					return nil, io.ErrUnexpectedEOF
				}
				return response(201, `{"keypair":{"name":"key","user_id":"owner","private_key":42}}`), nil
			})
			got, e := c.Keypairs.Create(context.Background(), compute.KeypairCreateRequest{Name: "key", UserID: "owner"}, types.CallOptions{})
			want := types.ProtocolError
			outcome := types.Accepted
			if loss {
				want = types.UnknownOutcome
				outcome = types.Unknown
			}
			if types.ErrorCode(e) != want || got.Receipt == nil || got.Receipt.Outcome != outcome || calls != 1 || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "key" || got.Receipt.ResourceRefs[0].Parent.ID != "owner" {
				t.Fatalf("key failure %+v %v", got, e)
			}
		})
	}
}
func TestCreateMicroversionConstraintsRejectBeforeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		change        func(*compute.CreateRequest)
	}{
		{"too-new", "2.94", func(*compute.CreateRequest) {}},
		{"tag-old", "2.51", func(r *compute.CreateRequest) { r.Options.Tags = []string{"tag"} }},
		{"hostname-old", "2.89", func(r *compute.CreateRequest) { r.Options.Hostname = "host" }},
		{"personality-removed", "2.57", func(r *compute.CreateRequest) { r.Personality = []compute.PersonalityFile{} }},
		{"net-tag-gap", "2.37", func(r *compute.CreateRequest) { r.Options.Networks = []servers.Network{{UUID: "network", Tag: "tag"}} }},
		{"bdm-tag-gap", "2.33", func(r *compute.CreateRequest) {
			r.Options.BlockDevice = []servers.BlockDevice{{SourceType: "blank", BootIndex: -1, Tag: "tag"}}
		}},
		{"volume-type-old", "2.66", func(r *compute.CreateRequest) {
			r.Options.BlockDevice = []servers.BlockDevice{{SourceType: "blank", BootIndex: -1, VolumeType: "fast"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := catalogClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.EOF })
			r := request()
			r.Microversion = tc.version
			tc.change(&r)
			got, e := c.Servers.Create(context.Background(), r)
			if types.ErrorCode(e) != types.InvalidArgument || got.Receipt == nil || got.Receipt.Outcome != types.NotSent || calls != 0 {
				t.Fatalf("version validation %+v %v calls%d", got, e, calls)
			}
		})
	}
}

func TestCreateLegacyBDMAndTrustedCertificatesWire(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		s := body["server"]
		if string(s["trusted_image_certificates"]) != "null" || string(s["host"]) != `"compute.example"` || !strings.Contains(string(s["block_device_mapping"]), `"no_device":false`) || !strings.Contains(string(s["block_device_mapping"]), `"connection_info":"private-connection"`) {
			t.Fatalf("legacy BDM wire %s", body["server"])
		}
		return response(202, `{"server":{"id":"created"}}`), nil
	})
	volume, host := "volume-id", "compute.example"
	noDevice := types.ProtectedJSON{ProtectedValue: types.Protect([]byte("false"))}
	connection := types.Protect([]byte("private-connection"))
	r := request()
	r.Microversion = "2.93"
	r.Host = &host
	r.TrustedImageCertificates = types.Null[[]string]()
	r.Options.Networks = "none"
	r.Options.ImageRef = ""
	r.LegacyBlockDevices = []compute.LegacyBlockDeviceMapping{{VolumeID: &volume, NoDevice: &noDevice, ConnectionInfo: &connection}}
	got, e := c.Servers.Create(context.Background(), r)
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("legacy BDM %+v %v", got, e)
	}
}

func TestKeypairCursorBindsOwnerAndResumesBoundedPages(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("limit") != "1" {
			t.Fatal("page bound missing")
		}
		switch r.URL.Query().Get("marker") {
		case "":
			return response(200, `{"keypairs":[{"keypair":{"name":"one","user_id":"owner"}}]}`), nil
		case "one":
			return response(200, `{"keypairs":[{"keypair":{"name":"two","user_id":"owner"}}]}`), nil
		case "two":
			return response(200, `{"keypairs":[]}`), nil
		}
		t.Fatal("unexpected marker")
		return nil, nil
	})
	first, e := c.Keypairs.Collect(context.Background(), compute.KeypairFilters{UserID: "owner"}, pagination.Options{PageSize: 1, MaxPages: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || first.NextCursor == nil || len(first.Items) != 1 {
		t.Fatalf("bounded keys %+v %v", first, e)
	}
	rest, e := c.Keypairs.Collect(context.Background(), compute.KeypairFilters{UserID: "owner"}, pagination.Options{PageSize: 1, Cursor: first.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || *rest.Items[0].Name != "two" {
		t.Fatalf("resumed %+v %v", rest, e)
	}
	if _, e := c.Keypairs.ListPage(context.Background(), compute.KeypairFilters{UserID: "other"}, pagination.Options{PageSize: 1, Cursor: first.NextCursor}); types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatalf("foreign cursor %v", e)
	}
}
func TestFlavorForeignNextLinkAndEmptyIdentityFailClosed(t *testing.T) {
	for _, body := range []string{`{"flavors":[{"id":""}]}`, `{"flavors":[],"flavors_links":[{"rel":"next","href":"https://foreign.invalid/flavors/detail?marker=x"}]}`} {
		c := catalogClient(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		if _, e := c.Flavors.ListPage(context.Background(), compute.FlavorFilters{}, pagination.Options{}); types.ErrorCode(e) != types.ProtocolError {
			t.Fatalf("bad flavor page %v", e)
		}
	}
}
func TestCreate293RejectsFQDNHostnameAndInvalidTrustedCertificates(t *testing.T) {
	c := catalogClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid create sent"); return nil, nil })
	for _, change := range []func(*compute.CreateRequest){func(r *compute.CreateRequest) { r.Options.Hostname = "vm.example" }, func(r *compute.CreateRequest) { r.TrustedImageCertificates = types.Value([]string{}) }, func(r *compute.CreateRequest) {
		r.TrustedImageCertificates = types.Value([]string{"duplicate", "duplicate"})
	}} {
		r := request()
		r.Microversion = "2.93"
		change(&r)
		got, e := c.Servers.Create(context.Background(), r)
		if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent {
			t.Fatalf("source constraints %+v %v", got, e)
		}
	}
}

func TestCreateNetworkUUIDConstraintStartsAt237(t *testing.T) {
	c := catalogClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("malformed modern UUID sent"); return nil, nil })
	r := request()
	r.Microversion = "2.37"
	got, e := c.Servers.Create(context.Background(), r)
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent {
		t.Fatalf("modern network %+v %v", got, e)
	}
}

func TestFlavorRepeatedSortParameters(t *testing.T) {
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.Join(r.URL.Query()["sort_key"], ",") != "memory_mb,vcpus" || strings.Join(r.URL.Query()["sort_dir"], ",") != "asc,desc" {
			t.Fatalf("sort query %s", r.URL.RawQuery)
		}
		return response(200, `{"flavors":[]}`), nil
	})
	_, e := c.Flavors.ListPage(context.Background(), compute.FlavorFilters{SortKeys: []string{"memory_mb", "vcpus"}, SortDirs: []string{"asc", "desc"}}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestComputeCatalogCreateAndSelectionBindings(t *testing.T) {
	c := catalog.Default()
	for _, id := range []string{"compute.v2_1.flavors.list", "compute.v2_1.flavors.get", "compute.v2_1.flavors_extra_specs.get", "compute.v2_1.keypairs.list", "compute.v2_1.keypairs.get", "compute.v2_1.keypairs.create", "compute.v2_1.keypairs.delete", "compute.v2_1.servers.create"} {
		v, e := c.Describe(id)
		if e != nil || v.Implementation.Status != "implemented" || v.GoMethod == nil || v.RequestBindingRef == nil || v.ResponseBindingRef == nil || v.ErrorBindingRef == nil || v.Auth.Scheme != types.IAMProject {
			t.Fatalf("binding %+v %v", v, e)
		}
	}
	for _, id := range []string{"compute.v2_1.servers.list", "compute.v2_1.servers.get", "compute.v2_1.servers.delete"} {
		v, e := c.Describe(id)
		if e != nil || v.Implementation.Status != "partial" {
			t.Fatalf("unrelated partial overclaim %+v %v", v, e)
		}
	}
	summary := c.Summary()
	if summary.InventoryComplete || summary.CoverageRatio != nil {
		t.Fatal("unknown Nova inventory overclaimed")
	}
}

func TestKeypairShortProviderPageDoesNotClaimCompletion(t *testing.T) {
	calls := 0
	c := catalogClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Query().Get("marker") {
		case "":
			return response(200, `{"keypairs":[{"keypair":{"name":"one","user_id":"owner"}}]}`), nil
		case "one":
			return response(200, `{"keypairs":[{"keypair":{"name":"two","user_id":"owner"}}]}`), nil
		default:
			return response(200, `{"keypairs":[]}`), nil
		}
	})
	got, e := c.Keypairs.Collect(context.Background(), compute.KeypairFilters{UserID: "owner"}, pagination.Options{PageSize: 100})
	if e != nil || !got.Complete || len(got.Items) != 2 || calls != 3 {
		t.Fatalf("clamped key pages %+v %v calls%d", got, e, calls)
	}
}

func TestCreatePreservesPresentEmptyProtectedPassword(t *testing.T) {
	c := catalogClient(t, func(*http.Request) (*http.Response, error) {
		return response(202, `{"server":{"id":"created","adminPass":""}}`), nil
	})
	got, e := c.Servers.Create(context.Background(), request())
	if e != nil || got.Data.AdminPassword == nil || len(got.Data.AdminPassword.Reveal()) != 0 {
		t.Fatalf("empty response password %+v %v", got, e)
	}
}

func TestCreateRejectsContradictoryResponseVariantWithReceipt(t *testing.T) {
	c := catalogClient(t, func(*http.Request) (*http.Response, error) {
		return response(202, `{"server":{"id":"created"},"reservation_id":"unexpected"}`), nil
	})
	got, e := c.Servers.CreateWithResult(context.Background(), request())
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "created" {
		t.Fatalf("contradictory union %+v %v", got, e)
	}
}

func TestKeypairTimestampKeepsMissingTimezoneAndExplicitInstant(t *testing.T) {
	for _, tc := range []struct{ wire, want, instant string }{
		{"2026-10-14T12:00:00.000000", "2026-10-14T12:00:00.000000", ""},
		{"2026-10-14T12:00:00.123456789123", "2026-10-14T12:00:00.123456789123", ""},
		{"2026-10-14T12:00:00", "2026-10-14T12:00:00", ""},
		{"2026-10-14T12:00:00.123456789+03:00", "2026-10-14T09:00:00.123456789Z", "2026-10-14T09:00:00.123456789Z"},
		{"2026-10-14T12:00:00Z", "2026-10-14T12:00:00Z", "2026-10-14T12:00:00Z"},
		{"2026-10-14T12:00:00-05:30", "2026-10-14T17:30:00Z", "2026-10-14T17:30:00Z"},
	} {
		t.Run(tc.wire, func(t *testing.T) {
			c := catalogClient(t, func(*http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{"keypair": map[string]any{"name": "key", "user_id": "owner", "created_at": tc.wire, "updated_at": tc.wire, "deleted_at": nil}})
				return response(200, string(body)), nil
			})
			reference := serverRef()
			reference.Kind = "keypair"
			reference.ID = "key"
			parent := reference
			parent.Kind = "keypair_user"
			parent.ID = "owner"
			reference.Parent = &parent
			got, e := c.Keypairs.Get(context.Background(), reference, types.CallOptions{})
			if e != nil || got.Data.CreatedAt == nil {
				t.Fatalf("timestamp response %+v %v", got, e)
			}
			raw, _ := json.Marshal(got.Data)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			want, _ := json.Marshal(tc.want)
			if string(fields["created_at"]) != string(want) || string(fields["updated_at"]) != string(want) || got.Data.CreatedAt.String() != tc.want {
				t.Fatalf("timestamp roundtrip %s expected %s", raw, want)
			}
			instant, ok := got.Data.CreatedAt.Instant()
			if tc.instant == "" {
				if ok || !instant.IsZero() {
					t.Fatalf("invented offset %v", instant)
				}
			} else if !ok || instant.Location() != time.UTC || instant.Format(time.RFC3339Nano) != tc.instant {
				t.Fatalf("wrong explicit instant %v %v", instant, ok)
			}
		})
	}
}
func TestKeypairMalformedTimestampReturnsProtocolError(t *testing.T) {
	for _, stamp := range []string{"not-a-time", "2026-13-14T12:00:00", "2026-10-14T25:00:00", "2026-10-14T12:00:00+not-offset", "2026-10-14T12:00:00+24:00", "2026-10-14T12:00:00+01:60"} {
		t.Run(stamp, func(t *testing.T) {
			c := catalogClient(t, func(*http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{"keypair": map[string]any{"name": "key", "user_id": "owner", "created_at": stamp}})
				return response(200, string(body)), nil
			})
			ref := serverRef()
			ref.Kind = "keypair"
			ref.ID = "key"
			parent := ref
			parent.Kind = "keypair_user"
			parent.ID = "owner"
			ref.Parent = &parent
			got, e := c.Keypairs.Get(context.Background(), ref, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || got.Data.CreatedAt != nil {
				t.Fatalf("malformed timestamp %+v %v", got, e)
			}
		})
	}
}
