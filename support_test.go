package selectelsdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/support"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const supportFileID = "f8e009a6-6911-41da-98d0-fee9e838b64d"

const supportTicketV2Body = `{"access_level":null,"categories":[],"created_at":"2026-10-15T00:00:00Z","is_await_client":false,"is_await_support":true,"is_can_be_voted":false,"is_client_read":true,"is_closed_by_system":false,"is_from_mailing":false,"keystone_projects":[],"notice_id":null,"notification_categories":[],"number":9007199254740993,"products":[],"split_from_ticket_number":null,"status":"new","summary":"synthetic-private-summary","unread_comments_count":0,"updated_at":null,"vote_created_at":null,"vote_score":null,"vote_type":null}`

func supportStaticSource() credentials.Source {
	return credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic-support", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-private-token"))}, nil
	})
}
func supportClient(t *testing.T, wire transportFunc) *sdk.Client {
	t.Helper()
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: supportStaticSource(), Transport: wire})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestSupportAttachmentMetadataUsesStaticAuthAndProtectsURLs(t *testing.T) {
	c := supportClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/files/public/files/uuid/"+supportFileID || r.Header.Get("X-Token") != "synthetic-private-token" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatal("attachment endpoint/auth")
		}
		return inventoryReply(200, `{"file_uuid":"`+supportFileID+`","original_filename":"synthetic-private-name","url":"https://synthetic-private.invalid/path?signature=secret","size":9007199254740993,"antivirus_checks":{"log":"synthetic-private-av","state":{"private":"synthetic-private-state"}},"state":{"private":"synthetic-private-state"},"ref":{"id":"forged"},"new_key":"synthetic-private-extra"}`), nil
	})
	svc := c.Account().Support().Attachments
	ref, e := svc.File(supportFileID)
	if e != nil {
		t.Fatal(e)
	}
	out, e := svc.GetMetadata(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || out.Data.Ref.ID != ref.ID || out.Data.Size.String() != "9007199254740993" {
		t.Fatal("typed file metadata", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private") {
		t.Fatal("file metadata leaked")
	}
}

func TestSupportAttachmentBatchAndFormatListsKeepExactFileTargetsAndBounds(t *testing.T) {
	c := supportClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			ids := r.URL.Query()["file_uuid"]
			if len(ids) != 2 || ids[0] != supportFileID || ids[1] != "37d58754-5dba-4bc3-a9ec-8945a2fc26a2" {
				t.Fatal("repeated UUID query")
			}
			return inventoryReply(200, `[{"file_uuid":"`+supportFileID+`"},{"file_uuid":"37d58754-5dba-4bc3-a9ec-8945a2fc26a2"}]`), nil
		}
		return inventoryReply(200, `{"items":["synthetic-private-first","synthetic-private-second"]}`), nil
	})
	svc := c.Account().Support().Attachments
	a, _ := svc.File(supportFileID)
	b, _ := svc.File("37d58754-5dba-4bc3-a9ec-8945a2fc26a2")
	batch, e := svc.GetMetadataBatch(context.Background(), []types.ResourceRef{a, b}, support.ReadBounds{MaxItems: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || len(batch.Data) != 1 || batch.Data[0].Ref == nil || batch.Data[0].Ref.ID != a.ID {
		t.Fatal("bounded exact metadata", e)
	}
	extensions, e := svc.Extensions(context.Background(), support.ReadBounds{MaxItems: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || len(*extensions.Data.Items) != 1 {
		t.Fatal("extensions bound", e)
	}
	mimetypes, e := svc.MIMETypes(context.Background(), support.ReadBounds{}, types.CallOptions{})
	if e != nil || len(*mimetypes.Data.Items) != 2 {
		t.Fatal("mimetypes", e)
	}
	raw, _ := json.Marshal([]any{extensions, mimetypes})
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("format-list output leak")
	}
}

func TestSupportAttachmentUploadStreamsOnlyDeclaredBytesAndRetainsAcceptedFile(t *testing.T) {
	reader := &ownedSupportReader{Reader: strings.NewReader("synthetic-private-payloadTRAILER")}
	c := supportClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/files/public/files/" || r.Header.Get("X-Token") == "" {
			t.Fatal("upload endpoint/auth")
		}
		if r.URL.Query().Get("actor_uuid") != "de359bf5-b5d6-4a92-8c70-836bc947b83a" {
			t.Fatal("explicit actor")
		}
		form, e := r.MultipartReader()
		if e != nil {
			t.Fatal(e)
		}
		part, e := form.NextPart()
		if e != nil || part.FormName() != "data" || part.FileName() != "synthetic-private.txt" {
			t.Fatal("multipart file field", e)
		}
		raw, e := io.ReadAll(part)
		if e != nil || string(raw) != "synthetic-private-payload" {
			t.Fatal("streamed payload length", e)
		}
		return inventoryReply(201, `{"file_uuid":"`+supportFileID+`","private_presigned_link":"https://synthetic-private.invalid?key=secret","size":25}`), nil
	})
	filename := types.Protect([]byte("synthetic-private.txt"))
	actor := "de359bf5-b5d6-4a92-8c70-836bc947b83a"
	out, e := c.Account().Support().Attachments.Upload(context.Background(), reader, 25, support.UploadOptions{Filename: &filename, ActorUUID: &actor}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != supportFileID || out.Data.Bytes != 25 || !out.Data.Complete || reader.closed {
		t.Fatal("stream ownership/receipt", e)
	}
	remaining, _ := io.ReadAll(reader)
	if string(remaining) != "TRAILER" {
		t.Fatal("reader probed beyond size")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("upload metadata leaked")
	}
}

type ownedSupportReader struct {
	io.Reader
	closed bool
}

func (r *ownedSupportReader) Close() error { r.closed = true; return nil }

func TestSupportAttachmentUploadProgressOnUnknownCancellationAndRejection(t *testing.T) {
	for _, mode := range []string{"unknown", "cancel", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			reader := &ownedSupportReader{Reader: strings.NewReader("synthetic-private-payload")}
			c := supportClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				var header []byte
				one := make([]byte, 1)
				for !bytes.HasSuffix(header, []byte("\r\n\r\n")) {
					if _, e := io.ReadFull(r.Body, one); e != nil {
						t.Fatal(e)
					}
					header = append(header, one[0])
					if len(header) > 8192 {
						t.Fatal("multipart header bound")
					}
				}
				if _, e := io.CopyN(io.Discard, r.Body, 4); e != nil {
					t.Fatal(e)
				}
				if mode == "rejected" {
					return inventoryReply(422, `{"message":"synthetic-private-error"}`), nil
				}
				if mode == "cancel" {
					cancel()
				}
				return nil, context.Canceled
			})
			out, e := c.Account().Support().Attachments.Upload(ctx, reader, 25, support.UploadOptions{}, types.CallOptions{})
			code, outcome := types.UnknownOutcome, types.Unknown
			if mode == "rejected" {
				code, outcome = types.ProviderError, types.Rejected
			}
			if types.ErrorCode(e) != code || out.Receipt == nil || out.Receipt.Outcome != outcome || out.Receipt.Attempts != 1 || calls != 1 || out.Data.Bytes != 4 || out.Data.Complete || reader.closed || len(out.Receipt.ResourceRefs) != 0 {
				t.Fatal("upload progress/no replay/ownership", e, out.Data.Bytes)
			}
		})
	}
}

func TestSupportTicketGetKeepsV2IdentityAndProtectedNullableFields(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v2", Scope: scope}: {ID: "explicit-tickets", URL: "https://tickets.example.invalid", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "tickets.example.invalid" || r.URL.Path != "/v2/tickets/9007199254740993" || r.Header.Get("X-Token") == "" {
			t.Fatal("ticket generation/auth")
		}
		return inventoryReply(200, supportTicketV2Body), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support().Tickets
	number, _ := types.ParseInteger("9007199254740993")
	ref, e := svc.Ticket(number)
	if e != nil {
		t.Fatal(e)
	}
	out, e := svc.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || out.Data.Ref.ID != number.String() || out.Data.AccessLevel == nil || out.Data.IsAwaitClient == nil || *out.Data.IsAwaitClient {
		t.Fatal("typed nullable ticket", e)
	}
	if _, valid := out.Data.AccessLevel.Value(); valid {
		t.Fatal("explicit null lost")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private") {
		t.Fatal("ticket output leaked")
	}
}

func TestSupportAttachmentListRetainsFilteredPartialPageWithoutInventedEnd(t *testing.T) {
	c := supportClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/files/public/files/list" || r.URL.Query().Get("page") != "3" || r.URL.Query().Get("page_size") != "1" || len(r.URL.Query()["external_id"]) != 2 || len(r.URL.Query()["user_id"]) != 2 || r.URL.Query().Get("state") != "synthetic-private-state" {
			t.Fatal("attachment list filters")
		}
		return inventoryReply(200, `[{"file_uuid":"`+supportFileID+`","original_filename":"synthetic-private-name"},{"file_uuid":"37d58754-5dba-4bc3-a9ec-8945a2fc26a2"}]`), nil
	})
	page, _ := types.ParseInteger("3")
	user, _ := types.ParseInteger("123")
	users := []types.Integer{user, user}
	ids := []types.ProtectedValue{types.Protect([]byte("synthetic-private-one")), types.Protect([]byte("synthetic-private-two"))}
	state := types.Protect([]byte("synthetic-private-state"))
	out, e := c.Account().Support().Attachments.List(context.Background(), support.AttachmentListOptions{Page: &page, UserIDs: &users, ExternalIDs: &ids, State: &state}, pagination.Options{PageSize: 10, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Complete || out.NextCursor == nil {
		t.Fatal("bounded partial files", e)
	}
}

func TestSupportTicketCreateRevealsTypedNullablePayloadOnlyOnWireAndRetainsIdentity(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v2", Scope: scope}: {ID: "explicit-tickets", URL: "https://tickets.example.invalid", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2/tickets" {
			t.Fatal("current create")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["summary"]) != `"synthetic-private-summary"` || string(body["comment_body"]) != `"synthetic-private-comment"` || !strings.Contains(string(body["body_values"]), "synthetic-private-value") || string(body["files"]) != "[]" || string(body["secrets"]) != "[]" {
			t.Fatal("typed protected body")
		}
		return inventoryReply(200, `{"number":9007199254740993,"summary":42}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	level := support.AccessLevelKeysEnum("all_users")
	summary := types.Protect([]byte("synthetic-private-summary"))
	comment := types.Protect([]byte("synthetic-private-comment"))
	empty := []types.ProtectedValue{}
	secrets := []types.Integer{}
	var opaque types.ProtectedJSON
	rawOpaque := []byte(`{"key":"synthetic-private-value"}`)
	json.Unmarshal(rawOpaque, &opaque)
	spec := support.CreateTicketRequestData{AccessLevel: &level, Summary: &summary, CommentBody: types.Value(comment), BodyValues: types.Value(opaque), Files: &empty, Categories: &empty, Products: &empty, KeystoneProjects: &empty, Secrets: &secrets}
	diagnostic := fmt.Sprintf("%+v %#v %+v %#v", spec, spec, *spec.BodyValues, *spec.BodyValues)
	if strings.Contains(diagnostic, "synthetic-private") || strings.Contains(diagnostic, fmt.Sprint(rawOpaque)) || strings.Contains(diagnostic, fmt.Sprintf("%#v", rawOpaque)) {
		t.Fatal("nullable protected storage leak")
	}
	out, e := c.Account().Support().Tickets.Create(context.Background(), spec, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "9007199254740993" || out.Receipt.Attempts != 1 {
		t.Fatal("accepted ticket identity", e)
	}
}

func TestSupportCurrentUpdateCountAndTaxonomyUseTypedShapes(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v2", Scope: scope}: {ID: "explicit-tickets", URL: "https://tickets.example.invalid", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v2/tickets/9007199254740993":
			if r.Method != "PATCH" {
				t.Fatal("update method")
			}
			var body map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&body)
			if string(body["access_level"]) != `"account_users"` || string(body["keystone_projects"]) != "[]" {
				t.Fatal("update values")
			}
			return inventoryReply(200, supportTicketV2Body), nil
		case "/v2/tickets/count":
			if r.URL.Query().Get("is_only_opened") != "false" || r.URL.Query().Get("q") != "synthetic-private-query" || len(r.URL.Query()["access_levels"]) != 2 {
				t.Fatal("count filters")
			}
			return inventoryReply(200, `{"items_count":9007199254740993}`), nil
		case "/v2/categories":
			if r.URL.Query().Get("is_only_active") != "false" {
				t.Fatal("category optional false")
			}
			return inventoryReply(200, `{"items":[{"id":1,"key":"key","names":null,"descriptions":null}]}`), nil
		case "/v2/categories/key/fields":
			return inventoryReply(200, `{"items":[{"key":"field","names":{"en":"synthetic-private-name"},"values":[{"key":"value","localizations":null}]}]}`), nil
		default:
			if r.URL.Query().Get("parent") != "synthetic-private-parent" {
				t.Fatal("product parent")
			}
			return inventoryReply(200, `{"items":[{"id":1,"key":"product","names":null}]}`), nil
		}
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support().Tickets
	number, _ := types.ParseInteger("9007199254740993")
	ref, _ := svc.Ticket(number)
	level := support.AccessLevelKeysEnum("account_users")
	projects := []types.ProtectedValue{}
	updated, e := svc.Update(context.Background(), ref, support.UpdateTicketRequest{AccessLevel: &level, KeystoneProjects: &projects}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted || updated.Data.Ref.ID != ref.ID {
		t.Fatal("update", e)
	}
	emptyQuery := types.Protect(nil)
	_, invalid := svc.Count(context.Background(), support.TicketFilters{Q: &emptyQuery}, types.CallOptions{})
	if types.ErrorCode(invalid) != types.InvalidArgument {
		t.Fatal("empty query violates source allowEmptyValue", invalid)
	}
	no := false
	q := types.Protect([]byte("synthetic-private-query"))
	levels := []support.AccessLevelKeysEnum{"all_users", "account_users"}
	count, e := svc.Count(context.Background(), support.TicketFilters{Q: &q, OnlyOpened: &no, AccessLevels: &levels}, types.CallOptions{})
	if e != nil || count.Data.ItemsCount.String() != "9007199254740993" {
		t.Fatal("count", e)
	}
	categories, e := svc.Categories(context.Background(), support.CategoryOptions{OnlyActive: &no}, pagination.Options{})
	if e != nil || !categories.Complete || len(categories.Items) != 1 || categories.Items[0].Names == nil {
		t.Fatal("categories", e)
	}
	fields, e := svc.CategoryFields(context.Background(), types.Protect([]byte("key")), pagination.Options{})
	if e != nil || len(fields.Items) != 1 {
		t.Fatal("fields", e)
	}
	parent := types.Protect([]byte("synthetic-private-parent"))
	products, e := svc.Products(context.Background(), support.ProductOptions{Parent: &parent}, pagination.Options{})
	if e != nil || !products.Complete || len(products.Items) != 1 {
		t.Fatal("products", e)
	}
	raw, _ := json.Marshal(fields)
	if strings.Contains(string(raw), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v %#v", fields.Items[0].Names, fields.Items[0].Names), "synthetic-private") {
		t.Fatal("nullable localized text leak")
	}
}

func TestSupportCurrentTicketListRetainsBoundedPageWithoutCountInference(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v2", Scope: scope}: {ID: "explicit-tickets", URL: "https://tickets.example.invalid", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v2/tickets" || r.URL.Query().Get("page") != "4" || r.URL.Query().Get("items_per_page") != "1" || r.URL.Query().Get("sort_by_order") != "asc" {
			t.Fatal("current list query")
		}
		second := strings.Replace(supportTicketV2Body, "9007199254740993", "9007199254740994", 1)
		return inventoryReply(200, `{"page":4,"items_per_page":1,"items":[`+supportTicketV2Body+`,`+second+`]}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	page, _ := types.ParseInteger("4")
	order := "asc"
	out, e := c.Account().Support().Tickets.List(context.Background(), support.TicketListOptions{Page: &page, SortOrder: &order}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || len(out.Items) != 1 || out.Items[0].Ref.ID != "9007199254740993" || out.Complete || out.NextCursor == nil || out.Envelopes[0].Items != nil || calls != 1 {
		t.Fatal("bounded current ticket page", e)
	}
}

func TestSupportLegacyTicketCommentAndVoteWorkflowKeepsExplicitGeneration(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v1", Scope: scope}: {ID: "explicit-legacy", URL: "https://legacy.example.invalid", APIGeneration: "v1", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "legacy.example.invalid" || r.Header.Get("X-Token") == "" {
			t.Fatal("legacy scope")
		}
		switch r.URL.Path {
		case "/tickets":
			if r.Method == "GET" {
				if r.URL.Query().Get("only_opened") != "false" {
					t.Fatal("legacy filter false")
				}
				return inventoryReply(200, `{"items":[{"number":42,"is_closed":false}],"page":1,"page_count":1,"items_per_page":1,"item_count":1}`), nil
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["comment_body"] != "synthetic-private-comment" || body["summary"] != "synthetic-private-summary" {
				t.Fatal("legacy creation body")
			}
		case "/tickets/42/comments":
			if r.Method == "GET" {
				return inventoryReply(200, `{"items":[{"id":7,"ticket_number":42,"body":"synthetic-private-comment"}]}`), nil
			}
			return inventoryReply(200, `{"id":7,"ticket_number":42,"body":"synthetic-private-comment"}`), nil
		case "/comments/7":
			return inventoryReply(200, `{"id":7,"ticket_number":42,"body":"synthetic-private-comment"}`), nil
		case "/tickets/42/vote":
			var body map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "PATCH" || string(body["score"]) != "8" {
				t.Fatal("published vote range/method")
			}
			return inventoryReply(200, `{"score":8,"comment":"synthetic-private-vote"}`), nil
		case "/comments/7/vote":
			return inventoryReply(200, `{"score":1}`), nil
		case "/topics":
			return inventoryReply(200, `{"items":[{"key":"general","names":{"en":"synthetic-private-topic"},"subtopics":[]}]}`), nil
		}
		return inventoryReply(200, `{"number":42,"is_closed":false,"summary":"synthetic-private-summary"}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support().Legacy
	summary := types.Protect([]byte("synthetic-private-summary"))
	body := types.Protect([]byte("synthetic-private-comment"))
	created, e := svc.Create(context.Background(), support.TicketCreateArgs{Summary: &summary, CommentBody: &body}, types.CallOptions{})
	if e != nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref == nil || created.Data.Ref.APIGeneration != "v1" {
		t.Fatal("legacy create", e)
	}
	ref := *created.Data.Ref
	got, e := svc.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref.ID != "42" {
		t.Fatal("legacy get", e)
	}
	modified, e := svc.Update(context.Background(), ref, support.TicketModifyArgs{}, types.CallOptions{})
	if e != nil || modified.Receipt.Outcome != types.Accepted {
		t.Fatal("legacy update", e)
	}
	no := false
	listed, e := svc.List(context.Background(), support.LegacyListOptions{OnlyOpened: &no}, pagination.Options{MaxItems: 1})
	if e != nil || !listed.Complete || len(listed.Items) != 1 || listed.Envelopes[0].Items != nil {
		t.Fatal("legacy source page count", e)
	}
	closed, e := svc.Close(context.Background(), ref, support.TicketCloseArgs{}, types.CallOptions{})
	if e != nil || closed.Receipt.Outcome != types.Accepted {
		t.Fatal("close", e)
	}
	reopened, e := svc.Reopen(context.Background(), ref, support.TicketReopenArgs{Body: &body}, types.CallOptions{})
	if e != nil || reopened.Receipt.Outcome != types.Accepted {
		t.Fatal("reopen", e)
	}
	comment, e := svc.AddComment(context.Background(), ref, support.CommentCreateArgs{Body: &body}, types.CallOptions{})
	if e != nil || comment.Data.Ref == nil || comment.Data.Ref.Parent == nil || comment.Data.Ref.Parent.ID != ref.ID {
		t.Fatal("comment caller parent", e)
	}
	readComment, e := svc.GetComment(context.Background(), *comment.Data.Ref, types.CallOptions{})
	if e != nil || readComment.Data.Ref.Parent.ID != ref.ID {
		t.Fatal("comment exact parent", e)
	}
	comments, e := svc.Comments(context.Background(), ref, pagination.Options{})
	if e != nil || !comments.Complete || len(comments.Items) != 1 {
		t.Fatal("comments", e)
	}
	eight, _ := types.ParseInteger("8")
	vote, e := svc.RateTicket(context.Background(), ref, support.TicketVoteCreateArgs{Score: &eight}, types.CallOptions{})
	if e != nil || vote.Receipt.Outcome != types.Accepted {
		t.Fatal("ticket vote", e)
	}
	one, _ := types.ParseInteger("1")
	rated, e := svc.RateComment(context.Background(), *comment.Data.Ref, support.CommentVoteCreateArgs{Score: &one}, types.CallOptions{})
	if e != nil || rated.Receipt.Outcome != types.Accepted {
		t.Fatal("comment vote", e)
	}
	topics, e := svc.Topics(context.Background(), pagination.Options{})
	if e != nil || !topics.Complete || len(topics.Items) != 1 || calls != 12 {
		t.Fatal("topics/workflow", e, calls)
	}
	raw, _ := json.Marshal([]any{got, readComment, comments, topics, vote})
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("support content leak")
	}
}

func TestSupportReceiptGoalsAndCommentParentDoNotInferReadinessOrOwnership(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	closedField := ""
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: supportStaticSource(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v1", Scope: scope}: {ID: "explicit-legacy", URL: "https://legacy.example.invalid", APIGeneration: "v1", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/comments") {
			return inventoryReply(200, `{"id":7,"ticket_number":42}`), nil
		}
		if strings.HasPrefix(r.URL.Path, "/comments/") {
			return inventoryReply(200, `{"id":7,"ticket_number":42}`), nil
		}
		return inventoryReply(200, `{"number":42`+closedField+`}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support().Legacy
	number, _ := types.ParseInteger("42")
	ref, _ := svc.Ticket(number)
	h, e := c.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	missing, e := h.Wait(context.Background(), "ticket_closed")
	if types.ErrorCode(e) != types.StateUnknown || missing.Complete {
		t.Fatal("missing closed flag completed wait", e)
	}
	closed, e := svc.Close(context.Background(), ref, support.TicketCloseArgs{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	incompatible := *closed.Receipt
	incompatible.ContractRevision = "incompatible"
	_, revisionErr := c.Account().Support().Resume(incompatible)
	if types.ErrorCode(revisionErr) != types.CapabilityUnavailable {
		t.Fatal("service resume revision", revisionErr)
	}
	h, e = c.Resume(*closed.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before := calls
	_, e = h.Wait(context.Background(), "ticket_not_closed")
	if types.ErrorCode(e) != types.CapabilityUnavailable || calls != before {
		t.Fatal("receipt exposed wrong goal", e)
	}
	closedField = `,"is_closed":true`
	done, e := h.Wait(context.Background(), "ticket_closed")
	if e != nil || !done.Complete {
		t.Fatal("closed goal", e)
	}
	body := types.Protect([]byte("synthetic-private-comment"))
	comment, e := svc.AddComment(context.Background(), ref, support.CommentCreateArgs{Body: &body}, types.CallOptions{})
	if e != nil || len(comment.Receipt.ResourceRefs) != 2 {
		t.Fatal("comment receipt anchors", e)
	}
	h, e = c.Resume(*comment.Receipt)
	if e != nil {
		t.Fatal("source-owned child selection", e)
	}
	done, e = h.Wait(context.Background(), "comment_observed")
	if e != nil || !done.Complete || done.Last.Ref.Parent == nil || done.Last.Ref.Parent.ID != ref.ID {
		t.Fatal("exact comment observation", e)
	}
	id, _ := types.ParseInteger("7")
	parentless, _ := svc.Comment(id, nil)
	read, e := svc.GetComment(context.Background(), parentless, types.CallOptions{})
	if e != nil || read.Data.Ref.Parent != nil || read.Data.TicketNumber.String() != "42" {
		t.Fatal("inferred comment parent", e)
	}
}

func TestSupportDownloadRefusesUnconfirmedSuccessContractBeforeIO(t *testing.T) {
	calls := 0
	c := supportClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		t.Fatal("unconfirmed download reached HTTP")
		return nil, nil
	})
	svc := c.Account().Support().Attachments
	ref, _ := svc.File(supportFileID)
	var destination bytes.Buffer
	out, e := svc.Download(context.Background(), ref, &destination, 1024, types.CallOptions{})
	if types.ErrorCode(e) != types.CapabilityUnavailable || calls != 0 || destination.Len() != 0 || out.Data.Complete || out.Data.Bytes != 0 || out.Data.Ref == nil || out.Data.Ref.ID != ref.ID {
		t.Fatal("unconfirmed download gate", e)
	}
}

func TestSupportAllMutationsKeepRevisionBoundNotSentReceiptsBeforeIO(t *testing.T) {
	credentialsCalls, httpCalls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
		credentialsCalls++
		return supportStaticSource().Lookup(ctx, r)
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { httpCalls++; return nil, context.Canceled })})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support()
	scope := types.Scope{AccountID: "test-account"}
	v1 := types.ResourceRef{Scope: scope, Service: "tickets", APIGeneration: "v1", Kind: "ticket", ID: "42", EndpointID: "explicit-v1", EndpointURL: "https://legacy.example.invalid"}
	v2 := v1
	v2.APIGeneration = "v2"
	comment := v1
	comment.Kind = "comment"
	comment.ID = "7"
	text := types.Protect([]byte("synthetic-private-body"))
	one, _ := types.ParseInteger("1")
	cases := []struct {
		name string
		call func() (*types.Receipt, error)
	}{
		{"v2_create", func() (*types.Receipt, error) {
			out, e := svc.Tickets.Create(context.Background(), support.CreateTicketRequestData{}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"v2_update", func() (*types.Receipt, error) {
			out, e := svc.Tickets.Update(context.Background(), v2, support.UpdateTicketRequest{}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"v1_create", func() (*types.Receipt, error) {
			out, e := svc.Legacy.Create(context.Background(), support.TicketCreateArgs{Summary: &text, CommentBody: &text}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"v1_update", func() (*types.Receipt, error) {
			out, e := svc.Legacy.Update(context.Background(), v1, support.TicketModifyArgs{}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"close", func() (*types.Receipt, error) {
			out, e := svc.Legacy.Close(context.Background(), v1, support.TicketCloseArgs{}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"reopen", func() (*types.Receipt, error) {
			out, e := svc.Legacy.Reopen(context.Background(), v1, support.TicketReopenArgs{Body: &text}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"comment", func() (*types.Receipt, error) {
			out, e := svc.Legacy.AddComment(context.Background(), v1, support.CommentCreateArgs{Body: &text}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"ticket_vote", func() (*types.Receipt, error) {
			out, e := svc.Legacy.RateTicket(context.Background(), v1, support.TicketVoteCreateArgs{Score: &one}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"comment_vote", func() (*types.Receipt, error) {
			out, e := svc.Legacy.RateComment(context.Background(), comment, support.CommentVoteCreateArgs{Score: &one}, types.CallOptions{})
			return out.Receipt, e
		}},
		{"upload", func() (*types.Receipt, error) {
			out, e := svc.Attachments.Upload(context.Background(), strings.NewReader(""), -1, support.UploadOptions{}, types.CallOptions{})
			return out.Receipt, e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receipt, e := tc.call()
			code := types.EndpointUnavailable
			if tc.name == "upload" {
				code = types.InvalidArgument
			}
			if types.ErrorCode(e) != code || receipt == nil || receipt.Outcome != types.NotSent || receipt.Attempts != 0 {
				t.Fatal("local receipt", e)
			}
			capability, x := c.Catalog().Describe(receipt.OperationID)
			if x != nil || receipt.ContractRevision == "" || receipt.ContractRevision != capability.ContractRevision {
				t.Fatal("local receipt lost contract revision", x)
			}
		})
	}
	if credentialsCalls != 0 || httpCalls != 0 {
		t.Fatal("local validation performed I/O")
	}
}

type supportCancelResponse struct {
	io.Reader
	cancel context.CancelFunc
	closed atomic.Int32
}

func (r *supportCancelResponse) Read(p []byte) (int, error) {
	n, e := r.Reader.Read(p)
	if n > 0 {
		r.cancel()
	}
	return n, e
}
func (r *supportCancelResponse) Close() error { r.closed.Add(1); return nil }
func TestSupportUploadCancellationOwnsResponseCloseOnceAndKeepsAcceptedIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responseBody := &supportCancelResponse{Reader: strings.NewReader(`{"file_uuid":"` + supportFileID + `","size":25}`), cancel: cancel}
	reader := &ownedSupportReader{Reader: strings.NewReader("synthetic-private-payload")}
	c := supportClient(t, func(r *http.Request) (*http.Response, error) {
		if _, e := io.Copy(io.Discard, r.Body); e != nil {
			t.Fatal(e)
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: responseBody}, nil
	})
	out, e := c.Account().Support().Attachments.Upload(ctx, reader, 25, support.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.Canceled || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != supportFileID || out.Data.Bytes != 25 || out.Data.Complete || reader.closed || responseBody.closed.Load() != 1 {
		t.Fatal("cancelled response ownership/receipt", e, responseBody.closed.Load())
	}
}

func TestSupportExplicitSchemaIAMVariantUsesAccountScopeWithoutStaticFallback(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "tickets", APIGeneration: "v2", Scope: scope}: {ID: "explicit-tickets", URL: "https://tickets.example.invalid", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/identity/v3/auth/tokens" {
			requested := requestedScope(r)
			if requested.ProjectID != "" {
				t.Fatal("ticket IAM incorrectly project-scoped")
			}
			return response(requested, time.Now().Add(time.Hour), 201), nil
		}
		if r.Header.Get("X-Auth-Token") == "" || r.Header.Get("X-Token") != "" {
			t.Fatal("IAM variant silently changed auth")
		}
		return inventoryReply(200, supportTicketV2Body), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := c.Account().Support().Tickets
	number, _ := types.ParseInteger("9007199254740993")
	ref, _ := svc.Ticket(number)
	out, e := svc.Get(context.Background(), ref, types.CallOptions{AuthMode: types.IAMAccount})
	if e != nil || out.Data.Ref == nil || out.Metadata.Scope.ProjectID != "" || calls != 2 {
		t.Fatal("explicit IAM wire variant", e)
	}
}

func TestSupportUploadLargeConfiguredJSONLimitDoesNotOverflow(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: supportStaticSource(), MaxJSONBytes: math.MaxUint64, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if _, e := io.Copy(io.Discard, r.Body); e != nil {
			t.Fatal(e)
		}
		return inventoryReply(201, `{"file_uuid":"`+supportFileID+`"}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Support().Attachments.Upload(context.Background(), strings.NewReader("file"), 4, support.UploadOptions{}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || !out.Data.Complete || out.Data.File.Ref == nil {
		t.Fatal("configured limit overflow", e)
	}
}
