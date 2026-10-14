package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/festy23/selectel-sdk/backups"
	"github.com/festy23/selectel-sdk/types"
)

func TestBackupPlanGetUsesCurrentProjectPathAndPreservesRetention(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ru-3.cloud.api.selcloud.ru" || r.URL.Path != "/data-protect/v2/test-project/plans/p1" {
			t.Fatal("wrong backup generation/scope/path")
		}
		return inventoryReply(200, `{"backup_mode":"frequency","created_at":"2026-10-14T12:00:00Z","description":"synthetic-private-description","id":"p1","max_backups":900719925474099312345,"name":"plan","resources":[],"schedule_pattern":"0 0 * * *","schedule_type":"crontab","status":"suspended"}`), nil
	}, 0)
	service := regionalView(t, c, "ru-3").Backups()
	ref := types.ResourceRef{Scope: regionalView(t, c, "ru-3").Scope(), Service: "backups", APIGeneration: "v2", Kind: "plan", ID: "p1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	out, e := service.Plans.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.MaxBackups.String() != "900719925474099312345" || out.Data.Ref.ID != "p1" || string(out.Data.Description.Reveal()) != "synthetic-private-description" || *out.Data.Status != "suspended" {
		t.Fatal("plan contract lost", e)
	}
}

func TestBackupPlanCreateAndUpdatePreserveNullableBodyAndReceipt(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		var b map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			t.Fatal("invalid plan body")
		}
		if r.Method == "POST" {
			if string(b["max_backups"]) != "0" || string(b["description"]) != `"synthetic-private-description"` {
				t.Fatal("plan wire field lost")
			}
			return inventoryReply(201, `{"backup_mode":"full","created_at":"2026-10-14T12:00:00Z","description":"synthetic-private-description","id":"p1","max_backups":0,"name":"plan","resources":[],"schedule_pattern":"pattern","schedule_type":"calendar","status":"started"}`), nil
		}
		if r.Method == "PATCH" {
			if string(b["description"]) != "null" || string(b["max_backups"]) != "0" {
				t.Fatal("nullable patch changed")
			}
			if _, ok := b["name"]; ok {
				t.Fatal("omitted patch name sent")
			}
			return inventoryReply(204, ""), nil
		}
		t.Fatal("unexpected method")
		return nil, nil
	}, 0)
	service := regionalView(t, c, "ru-3").Backups()
	zero, _ := types.ParseInteger("0")
	out, e := service.Plans.Create(context.Background(), backups.PlanCreateSpec{BackupMode: managementPtr("full"), Description: managementPtr(types.Protect([]byte("synthetic-private-description"))), MaxBackups: &zero, Name: managementPtr(types.Protect([]byte("plan"))), Resources: []backups.Resource{}, SchedulePattern: managementPtr("pattern"), ScheduleType: managementPtr("calendar")}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || out.Data.Ref == nil {
		t.Fatal("create receipt lost", e)
	}
	patched, e := service.Plans.Update(context.Background(), *out.Data.Ref, backups.PlanUpdateSpec{Description: backups.Null[types.ProtectedValue](), MaxBackups: backups.Value(zero)}, types.CallOptions{})
	if e != nil || patched.Receipt.Outcome != types.Accepted || patched.Receipt.ResourceRefs[0].ID != "p1" {
		t.Fatal("patch acknowledgement lost", e)
	}
}

func TestBackupListPagesRetainProviderTotalWithoutInventedCompletion(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/data-protect/v2/test-project/plans/" || r.URL.Query().Get("name") != "" || r.URL.Query().Get("marker") != "last" || r.URL.Query().Get("limit") != "1" {
			t.Fatal("backup list query changed")
		}
		return inventoryReply(200, `{"plans":[],"total":900719925474099312345}`), nil
	}, 0)
	one, _ := types.ParseInteger("1")
	out, e := regionalView(t, c, "ru-3").Backups().Plans.ListPage(context.Background(), backups.PlansQuery{Limit: &one, Marker: managementPtr("last"), Name: managementPtr("")}, types.CallOptions{})
	if e != nil || out.Data.Total.String() != "900719925474099312345" || out.Data.Plans == nil {
		t.Fatal("provider total/empty page lost", e)
	}
}

const backupCheckpointFixture = `{"id":"c1","plan_id":"p1","created_at":"2026-10-14T12:00:00Z","status":null,"checkpoint_items":[{"id":"i1","checkpoint_id":"c1","created_at":"2026-10-14T12:00:00Z","status":"available","resource":{"id":"volume1","name":"private-volume-name","type":"OS::Cinder::Volume"},"chain_id":"chain1","is_incremental":false}]}`

func TestBackupCheckpointsHaveExactParentRefsAndDeleteReceipts(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return inventoryReply(200, backupCheckpointFixture), nil
		}
		if r.URL.Query().Get("chain_id") == "chain1" {
			return inventoryReply(200, `{"deleted":[`+backupCheckpointFixture+`]}`), nil
		}
		return inventoryReply(200, `{"id":"c1","status":"deleting"}`), nil
	}, 0)
	region := regionalView(t, c, "ru-3")
	service := region.Backups()
	ref := types.ResourceRef{Scope: region.Scope(), Service: "backups", APIGeneration: "v2", Kind: "checkpoint", ID: "c1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	out, e := service.Checkpoints.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref.Parent.ID != "p1" || out.Data.CheckpointItems[0].Ref.Parent.ID != "c1" || out.Data.Status == nil {
		t.Fatal("checkpoint parent identity lost", e)
	}
	deleted, e := service.Checkpoints.Delete(context.Background(), *out.Data.Ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || deleted.Receipt.ResourceRefs[0].ID != "c1" {
		t.Fatal("checkpoint delete receipt lost", e)
	}
	chain := ref
	chain.Kind = "checkpoint_chain"
	chain.ID = "chain1"
	deletedChain, e := service.Checkpoints.DeleteChain(context.Background(), chain, types.CallOptions{})
	if e != nil || deletedChain.Receipt.Outcome != types.Accepted || len(deletedChain.Receipt.ResourceRefs) != 2 {
		t.Fatal("chain deletion identities lost", e)
	}
}

func TestBackupRestoreIsExplicitAPIActionWithAcceptedScopeBoundRefs(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return inventoryReply(200, backupCheckpointFixture), nil
		}
		if r.URL.Path != "/data-protect/v2/test-project/restores/" {
			t.Fatal("wrong restore path")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["checkpoint_item_id"] != "i1" || body["restore_name"] != "private-restore-name" {
			t.Fatal("restore target/name lost")
		}
		return inventoryReply(201, `{"id":"restore1","project_id":"test-project","checkpoint_id":"c1","checkpoint_item_id":"i1","status":"in_progress","resource_type":"OS::Cinder::Volume","resource_id":"new-volume"}`), nil
	}, 0)
	region := regionalView(t, c, "ru-3")
	service := region.Backups()
	ref := types.ResourceRef{Scope: region.Scope(), Service: "backups", APIGeneration: "v2", Kind: "checkpoint", ID: "c1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	checkpoint, e := service.Checkpoints.Get(context.Background(), ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	out, e := service.Restores.Create(context.Background(), *checkpoint.Data.CheckpointItems[0].Ref, types.Protect([]byte("private-restore-name")), types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || *out.Data.Status != "in_progress" || out.Data.Ref.Kind != "restore" || *out.Data.ResourceID != "new-volume" || out.Data.Ref.Scope.ProjectID != "test-project" {
		t.Fatal("restore receipt/status/ref lost", e)
	}
}

func TestBackupPlanDeleteUnknownIsNotRetriedAndCheckpointPagesPreserveNullableItems(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "DELETE" {
			return nil, io.ErrUnexpectedEOF
		}
		return inventoryReply(200, `{"checkpoints":[`+backupCheckpointFixture+`],"total":1}`), nil
	}, 0)
	region := regionalView(t, c, "ru-3")
	service := region.Backups()
	page, e := service.Checkpoints.ListPage(context.Background(), backups.CheckpointsQuery{}, types.CallOptions{})
	if e != nil || page.Data.Total.String() != "1" || page.Data.Checkpoints[0].Ref.Parent.ID != "p1" {
		t.Fatal("checkpoint page lost", e)
	}
	ref := *page.Data.Checkpoints[0].Ref.Parent
	out, e := service.Plans.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || calls != 2 || out.Receipt.ResourceRefs[0].ID != "p1" {
		t.Fatal("delete replayed or receipt lost", e)
	}
}

func TestBackupSDKV2VariantKeepsItsPublishedRouteAndRetentionField(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/data-protect/v2/plans/" {
			t.Fatal("SDK variant silently switched to project path")
		}
		if r.Method == "POST" {
			var body map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["full_backups_amount"]) != "5" {
				t.Fatal("SDK retention field changed")
			}
			if _, ok := body["max_backups"]; ok {
				t.Fatal("SDK retention guessed as current alias")
			}
			return inventoryReply(201, `{"id":"legacy1","full_backups_amount":5,"name":"legacy-plan"}`), nil
		}
		return inventoryReply(200, `{"plans":[{"id":"legacy1","full_backups_amount":5}],"total":1}`), nil
	}, 0)
	sdkVariant := regionalView(t, c, "ru-3").Backups().SDKV2
	five, _ := types.ParseInteger("5")
	created, e := sdkVariant.PlanCreate(context.Background(), &backups.SDKPlan{FullBackupsAmount: &five, Name: managementPtr(types.Protect([]byte("legacy-plan")))}, types.CallOptions{})
	if e != nil || created.Receipt.Outcome != types.Accepted || created.Data.FullBackupsAmount.String() != "5" || created.Data.Ref.Kind != "sdk_plan" {
		t.Fatal("SDK variant lost", e)
	}
	page, e := sdkVariant.PlansPage(context.Background(), backups.SDKPlansQuery{}, types.CallOptions{})
	if e != nil || page.Data.Total.String() != "1" {
		t.Fatal("SDK list lost", e)
	}
}

func TestBackupSDKV2ExactLifecycleAndCheckpointPageRemainIndependent(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/data-protect/v2/checkpoints/" {
			return inventoryReply(200, `{"checkpoints":[],"total":0}`), nil
		}
		if r.URL.Path != "/data-protect/v2/plans/legacy1" {
			t.Fatal("SDK path switched generation")
		}
		switch r.Method {
		case "GET":
			return inventoryReply(200, `{"id":"legacy1","full_backups_amount":5}`), nil
		case "PATCH":
			var body map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["full_backups_amount"]) != "10" || string(body["resources"]) != "null" || string(body["name"]) != `""` {
				t.Fatal("pinned SDK zero encoding changed")
			}
			return inventoryReply(200, `{"id":"legacy1","full_backups_amount":10}`), nil
		case "DELETE":
			return inventoryReply(204, ""), nil
		}
		return nil, io.ErrUnexpectedEOF
	}, 0)
	region := regionalView(t, c, "ru-3")
	variant := region.Backups().SDKV2
	ref := types.ResourceRef{Scope: region.Scope(), Service: "backups", APIGeneration: "v2", Kind: "sdk_plan", ID: "legacy1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	read, e := variant.PlanGet(context.Background(), ref, types.CallOptions{})
	if e != nil || read.Data.Ref.ID != "legacy1" {
		t.Fatal("SDK exact get lost", e)
	}
	ten, _ := types.ParseInteger("10")
	updated, e := variant.PlanUpdate(context.Background(), ref, &backups.SDKPlanUpdateSpec{FullBackupsAmount: &ten}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal("SDK update receipt lost", e)
	}
	deleted, e := variant.PlanDelete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal("SDK delete receipt lost", e)
	}
	page, e := variant.CheckpointsPage(context.Background(), backups.SDKCheckpointsQuery{}, types.CallOptions{})
	if e != nil || page.Data.Checkpoints == nil || page.Data.Total.String() != "0" {
		t.Fatal("SDK checkpoint page lost", e)
	}
	ref.Kind = "plan"
	_, e = variant.PlanGet(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 4 {
		t.Fatal("current ref entered SDK variant", e)
	}
}

func TestBackupChainDeleteScansLaterTrustedIDsAndAcceptedMalformedPlanKeepsRef(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return inventoryReply(201, `{"id":"p1","created_at":"bad"}`), nil
		}
		return inventoryReply(200, `{"deleted":[{"id":false},`+backupCheckpointFixture+`]}`), nil
	}, 0)
	region := regionalView(t, c, "ru-3")
	service := region.Backups()
	chain := types.ResourceRef{Scope: region.Scope(), Service: "backups", APIGeneration: "v2", Kind: "checkpoint_chain", ID: "chain1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	out, e := service.Checkpoints.DeleteChain(context.Background(), chain, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != "c1" {
		t.Fatal("later deleted checkpoint identity lost", e)
	}
	zero, _ := types.ParseInteger("0")
	created, e := service.Plans.Create(context.Background(), backups.PlanCreateSpec{BackupMode: managementPtr("full"), Description: managementPtr(types.Protect(nil)), MaxBackups: &zero, Name: managementPtr(types.Protect(nil)), Resources: []backups.Resource{}, SchedulePattern: managementPtr("pattern"), ScheduleType: managementPtr("calendar")}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || created.Receipt.Outcome != types.Accepted || len(created.Receipt.ResourceRefs) != 1 || created.Receipt.ResourceRefs[0].ID != "p1" {
		t.Fatal("accepted plan ID lost", e)
	}
}

func TestBackupUpdateRejectsUnsupportedPublishedEnumsBeforeIO(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return inventoryReply(204, ""), nil }, 0)
	region := regionalView(t, c, "ru-3")
	ref := types.ResourceRef{Scope: region.Scope(), Service: "backups", APIGeneration: "v2", Kind: "plan", ID: "p1", EndpointID: "selectel-backups.v2:ru-3", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/data-protect/v2"}
	_, e := region.Backups().Plans.Update(context.Background(), ref, backups.PlanUpdateSpec{Status: backups.Value("invented-state")}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal("unsupported enum reached provider", e)
	}
}

func TestBackupOptionalProtectedFormattingAndNullableValues(t *testing.T) {
	value := backups.Value(types.Protect([]byte("synthetic-sensitive-bytes")))
	for _, pattern := range []string{"%v", "%+v", "%#v", "%s"} {
		if got := fmt.Sprintf(pattern, value); got != "[REDACTED]" {
			t.Fatal("optional protected formatting exposed bytes")
		}
	}
	var item backups.CheckpointItem
	if json.Unmarshal([]byte(`{"id":"i","checkpoint_id":"c","created_at":"2026-10-14T00:00:00Z","status":null,"resource":{"id":null,"name":null,"type":null},"is_incremental":false}`), &item) != nil || item.Validate() != nil {
		t.Fatal("nullable decode rejected")
	}
	if value, present := item.IsIncremental.Value(); !present || value {
		t.Fatal("false confused with null")
	}
	if _, present := item.Status.Value(); present {
		t.Fatal("null confused with value")
	}
}
