package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/types"
)

func dbInteger(t *testing.T, value string) *types.Integer {
	t.Helper()
	n, err := types.ParseInteger(value)
	if err != nil {
		t.Fatal(err)
	}
	return &n
}
func dbCreateRequest(t *testing.T) databases.CreateRequest {
	return databases.CreateRequest{Name: managementPtr("app-db"), NodeCount: dbInteger(t, "1"), TypeID: managementPtr("postgres-type"), SubnetID: managementPtr("app-subnet"), FlavorID: managementPtr("small")}
}

func TestDatabaseCreateReturnsAcceptedScopedRef(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Host != "ru-1.dbaas.selcloud.ru" || r.URL.Path != "/v1/datastores" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		var body struct {
			Datastore map[string]json.RawMessage `json:"datastore"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid request JSON")
		}
		for field, want := range map[string]string{"name": `"app-db"`, "node_count": "1", "type_id": `"postgres-type"`, "subnet_id": `"app-subnet"`, "flavor_id": `"small"`, "project_id": `"test-project"`} {
			if string(body.Datastore[field]) != want {
				t.Errorf("%s = %s; want %s", field, body.Datastore[field], want)
			}
		}
		if len(body.Datastore) != 6 {
			t.Error("omitted options sent as defaults")
		}
		return inventoryReply(201, `{"datastore":{"id":"db/one","project_id":"test-project","status":"PENDING_CREATE","ref":"untrusted"}}`), nil
	}, 0)
	result, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Receipt.Attempts != 1 || len(result.Receipt.ResourceRefs) != 1 {
		t.Fatal("create acceptance evidence missing")
	}
	ref := result.Data.Ref
	if ref == nil || ref.ID != "db/one" || ref.Service != "databases" || ref.Kind != "datastore" || ref.APIGeneration != "v1" || ref.Scope != regionalView(t, c, "ru-1").Scope() || ref.EndpointURL != "https://ru-1.dbaas.selcloud.ru/v1" {
		t.Fatalf("wrong resource ref: %+v", ref)
	}
	if result.Data.Status == nil || *result.Data.Status != "PENDING_CREATE" {
		t.Fatal("acceptance mistaken for readiness")
	}
}

func dbRef(t *testing.T) types.ResourceRef {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected I/O"); return nil, nil }, 0)
	return types.ResourceRef{Scope: regionalView(t, c, "ru-1").Scope(), Service: "databases", APIGeneration: "v1", Kind: "datastore", ID: "db/one", EndpointID: "selectel-databases.v1:ru-1", EndpointURL: "https://ru-1.dbaas.selcloud.ru/v1"}
}
func TestDatabaseGetPreservesExactIdentityAndProtectedData(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.EscapedPath() != "/v1/datastores/db%2Fone" || r.URL.RawQuery != "" {
			t.Fatalf("wrong exact read: %s %s", r.Method, r.URL)
		}
		return inventoryReply(200, `{"datastore":{"id":"db/one","project_id":"test-project","status":"ACTIVE","disk_used":900719925474099312345,"created_at":"2025-01-01T12:00:00","config":{"password":"synthetic-db-secret"},"connection":{"password":"synthetic-db-secret"},"enabled":false,"security_groups":[],"ref":"ignored"}}`), nil
	}, 0)
	got, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Get(context.Background(), dbRef(t), types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Receipt != nil || got.Data.Ref == nil || got.Data.Ref.ID != "db/one" || got.Data.DiskUsed.String() != "900719925474099312345" || *got.Data.CreatedAt != "2025-01-01T12:00:00" || got.Data.SecurityGroups == nil || got.Data.Enabled == nil || *got.Data.Enabled {
		t.Fatal("read lost data or identity")
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "synthetic-db-secret") || strings.Contains(fmt.Sprintf("%+v", got), "synthetic-db-secret") {
		t.Fatal("protected response disclosed")
	}
	if !strings.Contains(string(got.Data.Config.Reveal()), "synthetic-db-secret") {
		t.Fatal("explicit reveal lost data")
	}
}

func TestDatabaseResumeWaitsForExactResourceWithoutRecreating(t *testing.T) {
	posts, reads := 0, 0
	c := mksClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
			return inventoryReply(201, `{"datastore":{"id":"db/one","status":"PENDING_CREATE"}}`), nil
		}
		reads++
		if r.Method != "GET" || r.URL.EscapedPath() != "/v1/datastores/db%2Fone" {
			t.Fatal("wait did not read exact datastore")
		}
		state := "PENDING_CREATE"
		if reads > 1 {
			state = "ACTIVE"
		}
		return inventoryReply(200, `{"datastore":{"id":"db/one","status":"`+state+`"}}`), nil
	})
	created, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(created.Receipt)
	var receipt types.Receipt
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	h, err := c.Resume(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Fatal("Resume performed I/O")
	}
	waited, err := h.Wait(context.Background(), "datastore.active")
	if err != nil || !waited.Complete || waited.Last == nil || waited.Last.Classification != "succeeded" || waited.Receipt == nil || waited.Receipt.CallID != receipt.CallID || posts != 1 || reads != 2 {
		t.Fatalf("wait failed: %+v %v", waited, err)
	}
	observed, err := c.Resource(*created.Data.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Receipt() != nil {
		t.Fatal("observation invented receipt")
	}
	status, err := observed.Status(context.Background())
	if err != nil || status.ProviderState != "ACTIVE" {
		t.Fatal("resource status failed", err)
	}
	if _, err = observed.Recover(context.Background()); types.ErrorCode(err) != types.RecoveryUnavailable {
		t.Fatal("observation invented mutation evidence", err)
	}
}

func TestDatabaseCreateRejectsInvalidInputBeforeIO(t *testing.T) {
	cases := map[string]func(*databases.CreateRequest){
		"missing name":       func(r *databases.CreateRequest) { r.Name = nil },
		"empty name":         func(r *databases.CreateRequest) { r.Name = managementPtr("") },
		"name pattern":       func(r *databases.CreateRequest) { r.Name = managementPtr("bad/name") },
		"name length":        func(r *databases.CreateRequest) { r.Name = managementPtr(strings.Repeat("a", 256)) },
		"zero nodes":         func(r *databases.CreateRequest) { r.NodeCount = dbInteger(t, "0") },
		"negative retention": func(r *databases.CreateRequest) { r.BackupRetentionDays = dbInteger(t, "-1") },
		"foreign project":    func(r *databases.CreateRequest) { r.ProjectID = managementPtr("other-project") },
		"empty subnet":       func(r *databases.CreateRequest) { r.SubnetID = managementPtr("") },
		"empty type":         func(r *databases.CreateRequest) { r.TypeID = managementPtr("") },
		"missing disk size": func(r *databases.CreateRequest) {
			r.Disk = &databases.DatastoreCreateDisk{Type: managementPtr("network-ultra")}
		},
		"disk type": func(r *databases.CreateRequest) {
			r.Disk = &databases.DatastoreCreateDisk{Size: dbInteger(t, "1"), Type: managementPtr("unknown")}
		},
		"flavor range": func(r *databases.CreateRequest) {
			r.Flavor = &databases.DatastoreCreateFlavor{Disk: dbInteger(t, "1"), RAM: dbInteger(t, "0"), VCPUs: dbInteger(t, "1")}
		},
		"master range": func(r *databases.CreateRequest) {
			r.FloatingIPs = &databases.DatastoreCreateFloatingIPs{Master: dbInteger(t, "2"), Replica: dbInteger(t, "0")}
		},
		"replica range": func(r *databases.CreateRequest) {
			r.FloatingIPs = &databases.DatastoreCreateFloatingIPs{Master: dbInteger(t, "0"), Replica: dbInteger(t, "1")}
		},
		"pooler size": func(r *databases.CreateRequest) { r.Pooler = &databases.CreateRequestPooler{Size: dbInteger(t, "501")} },
		"pooler mode": func(r *databases.CreateRequest) {
			r.Pooler = &databases.CreateRequestPooler{Mode: managementPtr("invalid")}
		},
		"restore source": func(r *databases.CreateRequest) {
			r.Restore = &databases.CreateRequestRestore{DatastoreID: managementPtr("")}
		},
		"empty security groups":     func(r *databases.CreateRequest) { r.SecurityGroups = []string{} },
		"duplicate security groups": func(r *databases.CreateRequest) { r.SecurityGroups = []string{"sg", "sg"} },
		"scalar config": func(r *databases.CreateRequest) {
			r.Config = &types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`false`))}
		},
		"null config": func(r *databases.CreateRequest) {
			r.Config = &types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`null`))}
		},
		"invalid config": func(r *databases.CreateRequest) {
			r.Config = &types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`{`))}
		},
		"empty password":        func(r *databases.CreateRequest) { v := types.Protect(nil); r.RedisPassword = &v },
		"invalid utf8 password": func(r *databases.CreateRequest) { v := types.Protect([]byte{255}); r.RedisPassword = &v },
		"invalid integer with config": func(r *databases.CreateRequest) {
			r.NodeCount = new(types.Integer)
			r.Config = &types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`{}`))}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) {
				calls++
				return inventoryReply(201, `{"datastore":{"id":"unexpected"}}`), nil
			}, 0)
			request := dbCreateRequest(t)
			change(&request)
			result, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), request, types.CallOptions{})
			code := types.InvalidArgument
			if name == "foreign project" {
				code = types.ScopeMismatch
			}
			if types.ErrorCode(err) != code || calls != 0 || result.Receipt == nil || result.Receipt.Outcome != types.NotSent || result.Receipt.Attempts != 0 {
				t.Fatalf("invalid input sent or wrong evidence: %v, calls=%d", err, calls)
			}
		})
	}
}

func TestDatabaseCreatePreservesAllOptionalFieldsAndSecrets(t *testing.T) {
	request := dbCreateRequest(t)
	request.FlavorID = nil
	request.NodeCount = dbInteger(t, "3")
	request.BackupRetentionDays = dbInteger(t, "7")
	request.Flavor = &databases.DatastoreCreateFlavor{Disk: dbInteger(t, "9007199254740993"), DiskType: managementPtr("local"), RAM: dbInteger(t, "1024"), VCPUs: dbInteger(t, "2")}
	request.Disk = &databases.DatastoreCreateDisk{Size: dbInteger(t, "8"), Type: managementPtr("network-ultra")}
	request.FloatingIPs = &databases.DatastoreCreateFloatingIPs{Master: dbInteger(t, "0"), Replica: dbInteger(t, "2")}
	request.Pooler = &databases.CreateRequestPooler{Mode: managementPtr("session"), Size: dbInteger(t, "500")}
	request.Restore = &databases.CreateRequestRestore{DatastoreID: managementPtr("source-id"), TargetTime: managementPtr("2025-01-01T12:00:00")}
	request.LogPlatform = &databases.DatastoreEnableLogPlatform{LogGroup: managementPtr("db-logs")}
	request.SecurityGroups = []string{"sg-one"}
	request.Config = &types.ProtectedJSON{ProtectedValue: types.Protect([]byte(`{"password":"synthetic-config-secret","enabled":false,"count":0,"empty":"","nullable":null,"large":900719925474099312345}`))}
	password := types.Protect([]byte("synthetic-redis-secret"))
	request.RedisPassword = &password
	raw, _ := json.Marshal(request)
	if strings.Contains(string(raw), "synthetic-") || strings.Contains(fmt.Sprintf("%#v", request), "synthetic-") {
		t.Fatal("request secrets disclosed")
	}
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		var body struct {
			Datastore map[string]json.RawMessage `json:"datastore"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid JSON")
		}
		want := map[string]string{
			"name": `"app-db"`, "node_count": "3", "project_id": `"test-project"`, "subnet_id": `"app-subnet"`, "type_id": `"postgres-type"`,
			"backup_retention_days": "7", "flavor": `{"disk":9007199254740993,"disk_type":"local","ram":1024,"vcpus":2}`,
			"disk": `{"size":8,"type":"network-ultra"}`, "floating_ips": `{"master":0,"replica":2}`, "pooler": `{"mode":"session","size":500}`,
			"restore": `{"datastore_id":"source-id","target_time":"2025-01-01T12:00:00"}`, "log_platform": `{"log_group":"db-logs"}`, "security_groups": `["sg-one"]`,
			"redis_password": `"synthetic-redis-secret"`, "config": `{"password":"synthetic-config-secret","enabled":false,"count":0,"empty":"","nullable":null,"large":900719925474099312345}`,
		}
		if len(body.Datastore) != len(want) {
			t.Error("extra or missing create fields")
		}
		for field, value := range want {
			if string(body.Datastore[field]) != value {
				t.Errorf("wire field %s changed", field)
			}
		}
		return inventoryReply(201, `{"datastore":{"id":"restored-id","status":"PENDING_CREATE"}}`), nil
	}, 0)
	got, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), request, types.CallOptions{})
	if err != nil || got.Data.Ref == nil || got.Data.Ref.ID != "restored-id" {
		t.Fatal("full create failed", err)
	}
	if request.ProjectID != nil {
		t.Fatal("caller request mutated")
	}
}

func TestDatabaseCreateErrorsKeepReceiptAndNeverReplay(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		transportErr error
		code         types.Code
		outcome      types.Outcome
		hasRef       bool
	}{
		{"lost response", 0, "", io.ErrUnexpectedEOF, types.UnknownOutcome, types.Unknown, false},
		{"bad request", 400, `{"error":{"message":"synthetic-private-error"}}`, nil, types.ProviderError, types.Rejected, false},
		{"forbidden", 403, `{"error":{"message":"synthetic-private-error"}}`, nil, types.PermissionDenied, types.Rejected, false},
		{"server error", 503, "", nil, types.UnknownOutcome, types.Unknown, false},
		{"missing id", 201, `{"datastore":{"name":"missing-id"}}`, nil, types.ProtocolError, types.Accepted, false},
		{"malformed data with id", 201, `{"datastore":{"id":"known","node_count":"invalid"}}`, nil, types.ProtocolError, types.Accepted, true},
		{"foreign project", 201, `{"datastore":{"id":"foreign","project_id":"other"}}`, nil, types.ScopeMismatch, types.Accepted, false},
		{"missing envelope", 201, `{}`, nil, types.ProtocolError, types.Accepted, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) {
				calls++
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return inventoryReply(tc.status, tc.body), nil
			}, 0)
			result, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
			if types.ErrorCode(err) != tc.code || calls != 1 || result.Receipt == nil || result.Receipt.Outcome != tc.outcome || result.Receipt.Attempts != 1 || (len(result.Receipt.ResourceRefs) > 0) != tc.hasRef {
				t.Fatalf("wrong mutation evidence: %v %+v", err, result.Receipt)
			}
			if strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("provider error disclosed")
			}
			if tc.hasRef {
				h, e := c.Resume(*result.Receipt)
				if e != nil || h.Resource().ID != "known" {
					t.Fatal("accepted malformed reply cannot resume exact known ref", e)
				}
			}
			if tc.outcome == types.Unknown && !tc.hasRef {
				if _, e := c.Resume(*result.Receipt); types.ErrorCode(e) != types.RecoveryUnavailable {
					t.Fatal("lost create invented recovery", e)
				}
			}
		})
	}
}

func TestDatabaseWaitStatesAndReadFailuresPreserveAcceptance(t *testing.T) {
	for _, tc := range []struct {
		state          string
		status         int
		code           types.Code
		classification string
	}{
		{"ERROR", 200, types.OperationFailed, "failed"}, {"DEGRADED", 200, types.OperationFailed, "failed"},
		{"DISK_FULL", 200, types.OperationFailed, "failed"}, {"DELETED", 200, types.OperationFailed, "failed"},
		{"NEW_PROVIDER_STATE", 200, types.StateUnknown, "unknown"}, {"", 200, types.StateUnknown, "unknown"},
		{"", 404, types.NotFound, ""}, {"", 403, types.PermissionDenied, ""},
	} {
		t.Run(tc.state+fmt.Sprint(tc.status), func(t *testing.T) {
			posts, reads := 0, 0
			c := mksClient(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" {
					posts++
					return inventoryReply(201, `{"datastore":{"id":"db/one"}}`), nil
				}
				reads++
				return inventoryReply(tc.status, `{"datastore":{"id":"db/one","status":"`+tc.state+`"}}`), nil
			})
			created, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			h, err := c.Resume(*created.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			result, err := h.Wait(context.Background(), "datastore.active")
			if types.ErrorCode(err) != tc.code || result.Complete || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || posts != 1 || reads != 1 {
				t.Fatalf("state incorrectly classified: %+v %v", result, err)
			}
			if tc.classification != "" && (result.Last == nil || result.Last.ProviderState != tc.state || result.Last.Classification != tc.classification) {
				t.Fatal("provider observation lost")
			}
		})
	}
}

func TestDatabaseWaitCancellationAndDeadlineKeepReceipt(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			c := mksClient(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" {
					return inventoryReply(201, `{"datastore":{"id":"db/one"}}`), nil
				}
				calls++
				if deadline {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				cancel()
				return inventoryReply(200, `{"datastore":{"id":"db/one","status":"PENDING_CREATE"}}`), nil
			})
			created, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			h, err := c.Resume(*created.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
			}
			result, err := h.Wait(ctx, "datastore.active")
			code := types.Canceled
			if deadline {
				code = types.DeadlineExceeded
			}
			if types.ErrorCode(err) != code || result.Complete || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || calls != 1 {
				t.Fatalf("wait cancellation changed mutation evidence: %+v %v", result, err)
			}
		})
	}
}

func TestDatabaseRefAndResumePreflight(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		return inventoryReply(200, `{"datastore":{"id":"db/one"}}`), nil
	}, 0)
	for name, change := range map[string]func(*types.ResourceRef){
		"project":    func(r *types.ResourceRef) { r.Scope.ProjectID = "foreign" },
		"account":    func(r *types.ResourceRef) { r.Scope.AccountID = "foreign" },
		"kind":       func(r *types.ResourceRef) { r.Kind = "server" },
		"generation": func(r *types.ResourceRef) { r.APIGeneration = "v2" },
		"parent":     func(r *types.ResourceRef) { parent := *r; r.Parent = &parent },
		"location":   func(r *types.ResourceRef) { r.Scope.Pool = "ru-3" },
	} {
		t.Run(name, func(t *testing.T) {
			ref := dbRef(t)
			change(&ref)
			if _, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Get(context.Background(), ref, types.CallOptions{}); err == nil {
				t.Fatal("foreign or malformed ref allowed")
			}
		})
	}
	ref := dbRef(t)
	ref.EndpointURL = "https://foreign.example/v1"
	if _, err := c.Resource(ref); types.ErrorCode(err) != types.EndpointUnavailable {
		t.Fatal("Resource accepted foreign endpoint", err)
	}
	if calls != 0 {
		t.Fatal("invalid refs reached provider")
	}
	for _, body := range []string{`{"datastore":{"id":"other"}}`, `{"datastore":{"id":"db/one","project_id":"foreign"}}`} {
		c := inventoryClient(t, func(*http.Request) (*http.Response, error) { return inventoryReply(200, body), nil }, 0)
		result, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Get(context.Background(), dbRef(t), types.CallOptions{})
		if types.ErrorCode(err) != types.ScopeMismatch || result.Data.Ref != nil {
			t.Fatal("foreign response trusted", err)
		}
	}
}

func TestDatabaseReceiptCompatibilityAndUnknownOutcome(t *testing.T) {
	reads := 0
	c := mksClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return inventoryReply(201, `{"datastore":{"id":"db/one"}}`), nil
		}
		reads++
		return inventoryReply(200, `{"datastore":{"id":"db/one","status":"ACTIVE"}}`), nil
	})
	created, err := regionalView(t, c, "ru-1").Databases().V1.Datastores.Create(context.Background(), dbCreateRequest(t), types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*types.Receipt){
		"contract": func(r *types.Receipt) { r.ContractRevision = "unknown-revision" },
		"account":  func(r *types.Receipt) { r.Scope.AccountID = "foreign" },
		"endpoint": func(r *types.Receipt) {
			r.Endpoint.URL = "https://foreign.example/v1"
			r.ResourceRefs[0].EndpointURL = r.Endpoint.URL
		},
		"kind": func(r *types.Receipt) { r.ResourceRefs[0].Kind = "server" },
		"read operation": func(r *types.Receipt) {
			op, _ := c.Catalog().Describe("databases.v1.datastores.get")
			r.OperationID = op.OperationID
			r.ContractRevision = op.ContractRevision
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(created.Receipt)
			var receipt types.Receipt
			_ = json.Unmarshal(raw, &receipt)
			change(&receipt)
			if _, err := c.Resume(receipt); err == nil {
				t.Fatal("incompatible receipt resumed")
			}
		})
	}
	if reads != 0 {
		t.Fatal("Resume performed I/O")
	}
	unknown := *created.Receipt
	unknown.Outcome = types.Unknown
	h, err := c.Resume(unknown)
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.Wait(context.Background(), "datastore.active")
	if types.ErrorCode(err) != types.UnknownOutcome || result.Complete || result.Receipt.Outcome != types.Unknown || result.Last == nil || result.Last.ProviderState != "ACTIVE" || reads != 1 {
		t.Fatal("resource existence turned into proof of creation", err)
	}
	for _, outcome := range []types.Outcome{types.NotSent, types.Rejected} {
		receipt := *created.Receipt
		receipt.Outcome = outcome
		h, err := c.Resume(receipt)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.Wait(context.Background(), "datastore.active"); types.ErrorCode(err) != types.CapabilityUnavailable {
			t.Fatal("nonaccepted mutation waited", err)
		}
	}
	if reads != 1 {
		t.Fatal("nonaccepted mutation triggered observation")
	}
	h, err = c.Resource(*created.Data.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Wait(context.Background(), "datastore.deleted"); types.ErrorCode(err) != types.CapabilityUnavailable {
		t.Fatal("unsupported delete goal accepted", err)
	}
}

func TestDatabaseCatalogExposesCreateGetAndTheirWaitGoalOffline(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }, 0)
	for _, action := range []string{"create", "get"} {
		op, err := c.Catalog().Describe("databases.v1.datastores." + action)
		if err != nil || op.Implementation.Status != "implemented" || len(op.WaitGoals) != 1 || op.WaitGoals[0] != "datastore.active" || op.Auth.Scheme != types.IAMProject || op.ContractRevision == "" {
			t.Fatal("database contract not discoverable", err)
		}
		if !slices.Equal(op.ScopeRequirements, []string{"account", "project", "pool"}) {
			t.Fatal("database discovery omitted required pool", op.ScopeRequirements)
		}
		if op.RequestBindingRef == nil {
			t.Fatal("request binding missing")
		}
		if _, err = c.Catalog().Artifact(*op.RequestBindingRef); err != nil {
			t.Fatal("binding artifact missing", err)
		}
	}
	if calls != 0 {
		t.Fatal("offline discovery performed I/O")
	}
}
