package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/mail"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"strings"
	"testing"
	"time"
)

const mailResourceID = "448355c9-8c3a-4706-8ca4-ed2970fce631"

func mailTestClient(t *testing.T, wire transportFunc) *sdk.Client {
	t.Helper()
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project"}
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "email", APIGeneration: "v1", Scope: scope}: {ID: "explicit-mail", URL: "https://mail.example.invalid/v1", APIGeneration: "v1", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestMailResourceReadProtectsCredentialsAndPinsExactScope(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "mail.example.invalid" || r.URL.Path != "/v1/resources/"+mailResourceID || r.Header.Get("X-Auth-Token") == "" {
			t.Fatal("explicit endpoint/IAM")
		}
		return inventoryReply(200, `{"resource":{"id":"`+mailResourceID+`","project_id":"test-project","status":"active","login":"synthetic-private-login","password":"synthetic-private-password","dns_key":"synthetic-private-key","dedicated_sender":false,"ref":{"id":"forged"},"new_secret":"synthetic-private-extra"}}`), nil
	})
	svc := projectView(t, c).Mail()
	ref, e := svc.Resource(mailResourceID)
	if e != nil {
		t.Fatal(e)
	}
	out, e := svc.GetResource(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Resource == nil || out.Data.Resource.Ref == nil || out.Data.Resource.Ref.ID != ref.ID || out.Data.Resource.DedicatedSender == nil || *out.Data.Resource.DedicatedSender {
		t.Fatal("typed scoped resource", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private") {
		t.Fatal("credential diagnostic leak")
	}
}

func TestMailCreateRetainsAcceptedIdentityBeforeCredentialFieldDecode(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v1/resources" {
			t.Fatal("create endpoint")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "synthetic-private-name" {
			t.Fatal("explicit protected request")
		}
		return inventoryReply(201, `{"id":"`+mailResourceID+`","project_id":"test-project","password":42}`), nil
	})
	name := types.Protect([]byte("synthetic-private-name"))
	out, e := projectView(t, c).Mail().CreateResource(context.Background(), mail.CreateResourceRequest{Name: &name}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != mailResourceID || out.Receipt.Attempts != 1 {
		t.Fatal("accepted identity lost", e)
	}
}

func TestMailResourceMutationsKeepSourceMethodsAndUnknownOutcome(t *testing.T) {
	calls := 0
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != "PUT" || r.URL.Path != "/v1/resources/"+mailResourceID {
				t.Fatal("rename method")
			}
			return inventoryReply(200, `{"id":"`+mailResourceID+`","project_id":"test-project","name":"renamed"}`), nil
		case 2:
			if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/password") {
				t.Fatal("rotate method")
			}
			return inventoryReply(200, `{"id":"`+mailResourceID+`","password":"synthetic-private-new"}`), nil
		case 3:
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["project_id"] != "explicit-target" || !strings.HasSuffix(r.URL.Path, "/transfer") {
				t.Fatal("transfer target")
			}
			return inventoryReply(200, `{"id":"`+mailResourceID+`","project_id":"explicit-target"}`), nil
		default:
			if r.Method != "DELETE" {
				t.Fatal("delete method")
			}
			return nil, context.Canceled
		}
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	name := types.Protect([]byte("renamed"))
	renamed, e := svc.RenameResource(context.Background(), ref, mail.UpdateResourceRequest{Name: &name}, types.CallOptions{})
	if e != nil || renamed.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	rotated, e := svc.RotateCredentials(context.Background(), ref, types.CallOptions{})
	if e != nil || rotated.Data.Password == nil || rotated.Receipt.ResourceRefs[0].ID != ref.ID {
		t.Fatal(e)
	}
	target := "explicit-target"
	transferred, e := svc.TransferResource(context.Background(), ref, mail.TransferResourceRequest{ProjectID: &target}, types.CallOptions{})
	if e != nil || transferred.Receipt.Scope.ProjectID != "test-project" || transferred.Data.Ref.Scope.ProjectID != "test-project" {
		t.Fatal("transfer adopted new owner", e)
	}
	removed, e := svc.DeleteResource(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || removed.Receipt == nil || removed.Receipt.Outcome != types.Unknown || calls != 4 || removed.Receipt.Attempts != 1 || removed.Receipt.ResourceRefs[0].ID != ref.ID {
		t.Fatal("delete unknown replay", e)
	}
}

func TestMailDomainActionsKeepExplicitNameAndVerificationFields(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/check") {
			if r.URL.Query().Get("name") != "selected.example" {
				t.Fatal("check name")
			}
			return inventoryReply(200, `{"verification":{"dkim_status":true,"dmarc_status":false,"dns_status":true,"spf_status":true,"verification_status":false}}`), nil
		}
		if r.Method == "POST" {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "selected.example" {
				t.Fatal("link name")
			}
			return inventoryReply(201, `{"domain":"selected.example"}`), nil
		}
		if r.Method == "DELETE" {
			if r.URL.Query().Get("name") != "selected.example" {
				t.Fatal("unlink name")
			}
			return inventoryReply(204, ""), nil
		}
		t.Fatal("unexpected request")
		return nil, nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	domain := types.Protect([]byte("selected.example"))
	linked, e := svc.LinkDomain(context.Background(), ref, mail.CreateDomainRequest{Name: &domain}, types.CallOptions{})
	if e != nil || linked.Receipt.Outcome != types.Accepted || linked.Receipt.ResourceRefs[0].ID != ref.ID {
		t.Fatal(e)
	}
	verification, e := svc.CheckDomain(context.Background(), ref, domain, types.CallOptions{})
	if e != nil || verification.Data.Verification == nil || verification.Data.Verification.DMARCStatus == nil || *verification.Data.Verification.DMARCStatus {
		t.Fatal("verification false lost", e)
	}
	unlinked, e := svc.UnlinkDomain(context.Background(), ref, domain, types.CallOptions{})
	if e != nil || unlinked.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
}

func TestMailStopListActionsDoNotSendMailAndProtectQueryAndRecords(t *testing.T) {
	calls := 0
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/stop-lists") || r.URL.Query().Get("email") != "synthetic-private-recipient@example.invalid" {
			t.Fatal("stop list target")
		}
		if r.Method == "GET" {
			if r.URL.Query().Has("mail_from") {
				t.Fatal("search sender invented")
			}
			return inventoryReply(200, `{"status":"ok","bounces":[{"bounce":"synthetic-private-reason","date":"2024-02-20 15:09:49"}],"domains":["synthetic-private-domain"],"fbl_report":[],"user_unsubscribe":[]}`), nil
		}
		if r.URL.Query().Get("mail_from") != "synthetic-private-sender@example.invalid" {
			t.Fatal("stop sender missing")
		}
		return inventoryReply(204, ""), nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	sender := types.Protect([]byte("synthetic-private-sender@example.invalid"))
	recipient := types.Protect([]byte("synthetic-private-recipient@example.invalid"))
	added, e := svc.AddStopListEntry(context.Background(), ref, mail.StopListEntry{MailFrom: sender, Email: recipient}, types.CallOptions{})
	if e != nil || added.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	found, e := svc.SearchStopList(context.Background(), ref, recipient, mail.ReadBounds{}, types.CallOptions{})
	if e != nil || found.Data.Bounces == nil || len(*found.Data.Bounces) != 1 {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(found)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("stop list leak")
	}
	removed, e := svc.RemoveStopListEntry(context.Background(), ref, mail.StopListEntry{MailFrom: sender, Email: recipient}, types.CallOptions{})
	if e != nil || removed.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal(e)
	}
}

func TestMailQuotasAndBoundedStatisticsKeepExactCounters(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/quotas":
			return inventoryReply(200, `{"ses_resources":{"max":10,"used":3}}`), nil
		case "/v1/resources/" + mailResourceID + "/quotas":
			return inventoryReply(200, `{"domains":{"max":3,"used":0}}`), nil
		default:
			if r.URL.Query().Get("date_from") != "2024-05-25" || r.URL.Query().Get("date_to") != "2024-05-26" || r.URL.Query().Has("limit") {
				t.Fatal("statistics query")
			}
			return inventoryReply(200, `{"limit":2,"next_offset":0,"statistics":{"statistics_by_dates":[{"date":"2024-05-25","stat":{"send_ok":9007199254740993}},{"date":"2024-05-26","stat":{"send_ok":2}}],"total":{"send_ok":9007199254740995}}}`), nil
		}
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	quotas, e := svc.ProjectQuotas(context.Background(), types.CallOptions{})
	if e != nil || quotas.Data.SESResources.Used.String() != "3" {
		t.Fatal(e)
	}
	resource, e := svc.ResourceQuotas(context.Background(), ref, types.CallOptions{})
	if e != nil || resource.Data.Domains.Used.String() != "0" {
		t.Fatal(e)
	}
	stats, e := svc.GetStatistics(context.Background(), ref, mail.Period{DateFrom: "2024-05-25", DateTo: "2024-05-26"}, mail.ReadBounds{MaxItems: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || stats.Data.Statistics == nil || len(*stats.Data.Statistics.StatisticsByDates) != 1 || stats.Data.Statistics.Total.SendOK.String() != "9007199254740995" || (*stats.Data.Statistics.StatisticsByDates)[0].Stat.SendOK.String() != "9007199254740993" {
		t.Fatal("bounded exact statistics", e)
	}
}

func TestMailDeliveryStatusUsesExplicitIdentifiersAndBoundsMessages(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("message_id") != "synthetic-private-message" || r.URL.Query().Get("x_track_id") != "synthetic-private-track" {
			t.Fatal("delivery identifiers")
		}
		return inventoryReply(200, `{"status":"ok","messages":[{"created_at":9007199254740993,"message_id":"synthetic-private-message","reason":"synthetic-private-reason","status":"future-provider-state"},{"message_id":"second"}]}`), nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	id := types.Protect([]byte("synthetic-private-message"))
	track := types.Protect([]byte("synthetic-private-track"))
	out, e := svc.GetDeliveryStatus(context.Background(), ref, mail.DeliveryQuery{MessageID: &id, XTrackID: &track}, mail.ReadBounds{MaxItems: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || out.Data.Messages == nil || len(*out.Data.Messages) != 1 || (*out.Data.Messages)[0].CreatedAt.String() != "9007199254740993" || *(*out.Data.Messages)[0].Status != "future-provider-state" {
		t.Fatal("bounded message statuses", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("delivery telemetry leak")
	}
}

func TestMailResourceListBoundsRowsAndReplaysExactPartialPage(t *testing.T) {
	calls := 0
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		wantLimit := "1"
		if calls > 1 {
			wantLimit = "2"
		}
		if r.URL.Query().Get("offset") != "0" || r.URL.Query().Get("limit") != wantLimit {
			t.Fatal("list bounds query")
		}
		return inventoryReply(200, `{"limit":2,"next_offset":0,"resources":[{"id":"`+mailResourceID+`","project_id":"test-project","password":"synthetic-private-one"},{"id":"37d58754-5dba-4bc3-a9ec-8945a2fc26a2","password":"synthetic-private-two"}]}`), nil
	})
	svc := projectView(t, c).Mail()
	first, e := svc.ListResources(context.Background(), pagination.Options{PageSize: 2, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || len(first.Items) != 1 || first.Complete || first.NextCursor == nil || first.Envelopes[0].Resources != nil || first.Items[0].Ref == nil {
		t.Fatal("partial resource page", e)
	}
	next, e := svc.ListResources(context.Background(), pagination.Options{PageSize: 2, Cursor: first.NextCursor})
	if types.ErrorCode(e) != types.ProtocolError || next.Complete || len(next.Items) != 1 || next.Items[0].Ref.ID == mailResourceID {
		t.Fatal("unproven end or partial page lost", e)
	}
}

func TestMailDomainListAndReportsRetainBoundedSensitiveRows(t *testing.T) {
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/domains") {
			return inventoryReply(200, `{"limit":1,"next_offset":0,"domains":["synthetic-private-domain"]}`), nil
		}
		if r.URL.Query().Get("date_from") != "2024-05-25" || r.URL.Query().Get("date_to") != "2024-05-26" || r.URL.Query().Get("cursor_next") != "0" {
			t.Fatal("report query")
		}
		if strings.HasSuffix(r.URL.Path, "/fbl") {
			return inventoryReply(200, `{"status":"ok","pagination":{"cursor_next":0},"report":[{"issuen":9007199254740993,"email":"synthetic-private-recipient","x_track_id":"synthetic-private-track"}]}`), nil
		}
		return inventoryReply(200, `{"status":"ok","pagination":{"cursor_next":0},"report":[{"issuen":9007199254740993,"code":550,"message":"synthetic-private-reason"}]}`), nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	period := mail.Period{DateFrom: "2024-05-25", DateTo: "2024-05-26"}
	domains, e := svc.ListDomains(context.Background(), ref, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.ProtocolError || len(domains.Items) != 1 || domains.Complete || domains.Envelopes[0].Domains != nil {
		t.Fatal("domain incomplete page", e)
	}
	complaints, e := svc.ListComplaints(context.Background(), ref, period, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.ProtocolError || len(complaints.Items) != 1 || complaints.Items[0].Issuen.String() != "9007199254740993" || complaints.Envelopes[0].Report != nil {
		t.Fatal("complaint incomplete page", e)
	}
	undelivered, e := svc.ListUndelivered(context.Background(), ref, period, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.ProtocolError || len(undelivered.Items) != 1 || undelivered.Items[0].Code.String() != "550" {
		t.Fatal("undelivered incomplete page", e)
	}
	raw, _ := json.Marshal([]any{domains, complaints, undelivered})
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("report leak")
	}
}

func TestMailStopListResponseBoundsApplyAcrossAllCollections(t *testing.T) {
	c := mailTestClient(t, func(*http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"bounces":[{"bounce":"first"}],"domains":["second"],"fbl_report":[{"domain":"third"}],"user_unsubscribe":[{"domain":"fourth"}]}`), nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	out, e := svc.SearchStopList(context.Background(), ref, types.Protect([]byte("selected@example.invalid")), mail.ReadBounds{MaxItems: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || out.Data.Bounces == nil || len(*out.Data.Bounces) != 1 || out.Data.Domains == nil || len(*out.Data.Domains) != 0 || len(*out.Data.FblReport) != 0 || len(*out.Data.UserUnsubscribe) != 0 {
		t.Fatal("cross collection response limit", e)
	}
}

func TestMailAllMutationsRetainReceiptsOnEndpointAndUnknownFailures(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			credentialsCalls, httpCalls := 0, 0
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
				credentialsCalls++
				return source().Lookup(ctx, r)
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { httpCalls++; return nil, context.Canceled })})
			if e != nil {
				t.Fatal(e)
			}
			if configured {
				c = mailTestClient(t, func(*http.Request) (*http.Response, error) { httpCalls++; return nil, context.Canceled })
			}
			svc := projectView(t, c).Mail()
			scope := types.Scope{AccountID: "test-account", ProjectID: "test-project"}
			ref := types.ResourceRef{Service: "email", APIGeneration: "v1", Kind: "resource", ID: mailResourceID, Scope: scope, EndpointID: "explicit-mail", EndpointURL: "https://mail.example.invalid/v1"}
			value := types.Protect([]byte("selected@example.invalid"))
			target := "target"
			cases := []struct {
				name string
				call func() (*types.Receipt, error)
			}{
				{"create", func() (*types.Receipt, error) {
					out, e := svc.CreateResource(context.Background(), mail.CreateResourceRequest{Name: &value}, types.CallOptions{})
					return out.Receipt, e
				}},
				{"rename", func() (*types.Receipt, error) {
					out, e := svc.RenameResource(context.Background(), ref, mail.UpdateResourceRequest{Name: &value}, types.CallOptions{})
					return out.Receipt, e
				}},
				{"rotate", func() (*types.Receipt, error) {
					out, e := svc.RotateCredentials(context.Background(), ref, types.CallOptions{})
					return out.Receipt, e
				}},
				{"transfer", func() (*types.Receipt, error) {
					out, e := svc.TransferResource(context.Background(), ref, mail.TransferResourceRequest{ProjectID: &target}, types.CallOptions{})
					return out.Receipt, e
				}},
				{"delete", func() (*types.Receipt, error) {
					out, e := svc.DeleteResource(context.Background(), ref, types.CallOptions{})
					return out.Receipt, e
				}},
				{"link", func() (*types.Receipt, error) {
					out, e := svc.LinkDomain(context.Background(), ref, mail.CreateDomainRequest{Name: &value}, types.CallOptions{})
					return out.Receipt, e
				}},
				{"unlink", func() (*types.Receipt, error) {
					out, e := svc.UnlinkDomain(context.Background(), ref, value, types.CallOptions{})
					return out.Receipt, e
				}},
				{"stop_add", func() (*types.Receipt, error) {
					out, e := svc.AddStopListEntry(context.Background(), ref, mail.StopListEntry{MailFrom: value, Email: value}, types.CallOptions{})
					return out.Receipt, e
				}},
				{"stop_remove", func() (*types.Receipt, error) {
					out, e := svc.RemoveStopListEntry(context.Background(), ref, mail.StopListEntry{MailFrom: value, Email: value}, types.CallOptions{})
					return out.Receipt, e
				}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					receipt, e := tc.call()
					code, outcome, attempts := types.EndpointUnavailable, types.NotSent, uint(0)
					if configured {
						code, outcome, attempts = types.UnknownOutcome, types.Unknown, 1
					}
					if types.ErrorCode(e) != code || receipt == nil || receipt.Outcome != outcome || receipt.Attempts != attempts {
						t.Fatal("missing endpoint receipt", e)
					}
				})
			}
			wantCalls := 0
			if configured {
				wantCalls = 9
			}
			if credentialsCalls != 0 || httpCalls != wantCalls {
				t.Fatal("endpoint validation did I/O")
			}
		})
	}
}

func TestMailExactResourceObservationKeepsUnknownStateAndReceiptGoalBoundary(t *testing.T) {
	state := "future_state"
	c := mailTestClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			return inventoryReply(201, `{"id":"`+mailResourceID+`","status":"active"}`), nil
		}
		if r.Method == "DELETE" {
			return inventoryReply(204, ""), nil
		}
		if state == "absent" {
			return inventoryReply(404, `{}`), nil
		}
		return inventoryReply(200, `{"resource":{"id":"`+mailResourceID+`","status":"`+state+`"}}`), nil
	})
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	h, e := c.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := h.Status(context.Background())
	if e != nil || observed.Classification != "unknown" || observed.ProviderState != "future_state" {
		t.Fatal("unknown state", e)
	}
	name := types.Protect([]byte("selected"))
	created, e := svc.CreateResource(context.Background(), mail.CreateResourceRequest{Name: &name}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	h, e = c.Resume(*created.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	_, e = h.Wait(context.Background(), "resource_absent")
	if types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal("create exposed delete goal", e)
	}
	state = "active"
	done, e := h.Wait(context.Background(), "resource_active")
	if e != nil || !done.Complete {
		t.Fatal("active exact resource", e)
	}
	deleted, e := svc.DeleteResource(context.Background(), ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	h, e = c.Resume(*deleted.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	_, e = h.Wait(context.Background(), "resource_active")
	if types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal("delete exposed create goal", e)
	}
	state = "absent"
	done, e = h.Wait(context.Background(), "resource_absent")
	if e != nil || !done.Complete {
		t.Fatal("absent exact resource", e)
	}
}

func TestMailStopListEmailFormatValidationPrecedesIO(t *testing.T) {
	c := mailTestClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid email reached HTTP"); return nil, nil })
	svc := projectView(t, c).Mail()
	ref, _ := svc.Resource(mailResourceID)
	out, e := svc.AddStopListEntry(context.Background(), ref, mail.StopListEntry{MailFrom: types.Protect([]byte("sender@example.invalid")), Email: types.Protect([]byte("not-an-address"))}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.Attempts != 0 {
		t.Fatal("source email validation", e)
	}
}
