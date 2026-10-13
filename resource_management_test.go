package selectelsdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/projects"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/types"
)

func managementPtr[T any](v T) *T { return &v }
func projectRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "projects", APIGeneration: "v2", Kind: "project", ID: "p/one", EndpointID: "selectel-resell", EndpointURL: "https://api.selectel.ru/vpc/resell"}
}
func registryRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project"}, Service: "registry", APIGeneration: "v1", Kind: "registry", ID: "r/one", EndpointID: "selectel-registry", EndpointURL: "https://cr.selcloud.ru/api"}
}

func TestProjectLifecycleAndNullableUpdates(t *testing.T) {
	name := strings.Repeat("п", 64)
	requests := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "api.selectel.ru" {
			t.Error("incorrect project endpoint")
		}
		if r.Method == "POST" {
			if r.URL.Path != "/vpc/resell/v2/projects" {
				t.Error("incorrect create path")
			}
			var body struct {
				Project map[string]json.RawMessage `json:"project"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || string(body.Project["skip_quotas_init"]) != "false" || string(body.Project["description"]) != `""` {
				t.Error("optional false/empty create values lost")
			}
			return inventoryReply(200, `{"project":{"id":"p/one","name":"provider-name","url":"provider-url","ref":"ignored-provider-field"}}`), nil
		}
		if r.URL.EscapedPath() != "/vpc/resell/v2/projects/p%2Fone" {
			t.Error("project ID not escaped as one segment")
		}
		switch r.Method {
		case "GET":
			if r.URL.Query().Get("exclude_quotas") != "false" {
				t.Error("explicit false get filter lost")
			}
			return inventoryReply(200, `{"project":{"id":"p/one","enabled":false,"quotas":{"compute.cores":[{"region":"ru-1","zone":null,"value":900719925474099312345,"used":0}]},"theme":{"brand_color":"","logo":"logo"}}}`), nil
		case "PATCH":
			var body struct {
				Project map[string]json.RawMessage `json:"project"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("malformed update body")
			}
			if _, ok := body.Project["name"]; ok {
				t.Error("omitted name sent")
			}
			if string(body.Project["custom_url"]) != "null" || string(body.Project["description"]) != `""` {
				t.Error("nullable update presence lost")
			}
			var theme map[string]json.RawMessage
			if json.Unmarshal(body.Project["theme"], &theme) != nil || string(theme["color"]) != "null" || string(theme["brand_color"]) != `""` {
				t.Error("nested nullable update fields lost")
			}
			if _, ok := theme["logo"]; ok {
				t.Error("omitted nested logo sent")
			}
			return inventoryReply(201, `{"project":{"id":"p/one","name":"updated","enabled":false,"theme":{"color":""}}}`), nil
		case "DELETE":
			return inventoryReply(204, ""), nil
		default:
			t.Fatal("unexpected project method")
			return nil, nil
		}
	}, 0)
	p := c.Account().Projects()
	created, err := p.Create(context.Background(), projects.CreateRequest{Name: name, Description: managementPtr(""), SkipQuotasInit: managementPtr(false)}, types.CallOptions{})
	if err != nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref == nil || created.Data.Ref.ID != "p/one" || created.Data.Ref.Scope.ProjectID != "" {
		t.Fatal("project create failed", err)
	}
	ref := *created.Data.Ref
	read, err := p.Get(context.Background(), ref, projects.GetOptions{ExcludeQuotas: managementPtr(false)}, types.CallOptions{})
	if err != nil || read.Receipt != nil || read.Data.Enabled == nil || *read.Data.Enabled || read.Data.Quotas["compute.cores"][0].Value.String() != "900719925474099312345" {
		t.Fatal("project detail fields lost", err)
	}
	updated, err := p.Update(context.Background(), projects.UpdateRequest{Ref: ref, Description: managementPtr(""), CustomURL: types.Null[string](), Theme: &projects.ThemeRequest{Color: types.Null[string](), BrandColor: types.Value("")}}, types.CallOptions{})
	if err != nil || updated.Receipt.Outcome != types.Accepted || updated.Data.Theme == nil || updated.Data.Ref.ID != ref.ID {
		t.Fatal("project update failed", err)
	}
	deleted, err := p.Delete(context.Background(), ref, types.CallOptions{})
	if err != nil || deleted.Receipt.Outcome != types.Accepted || requests != 4 {
		t.Fatal("project delete failed", err)
	}
}

func TestProjectStatusBindingsAndAcceptedDecodeFailure(t *testing.T) {
	for _, tc := range []struct {
		action  string
		status  int
		body    string
		code    types.Code
		outcome types.Outcome
		knownID bool
	}{
		{"get", 400, `{"message":"synthetic-private-provider-message"}`, types.NotFound, "", false},
		{"delete", 400, "", types.NotFound, types.Rejected, true},
		{"create", 400, "", types.ProviderError, types.Rejected, false},
		{"update", 409, "", types.Conflict, types.Rejected, true},
		{"create", 200, `{"project":{"id":"known","name":123}}`, types.ProtocolError, types.Accepted, true},
		{"create", 200, `{"project":{"name":"missing-id"}}`, types.ProtocolError, types.Accepted, false},
		{"get", 200, `{"project":{"id":"foreign"}}`, types.ScopeMismatch, "", false},
	} {
		t.Run(tc.action+string(tc.outcome)+string(tc.code), func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return inventoryReply(tc.status, tc.body), nil }, 0)
			p := c.Account().Projects()
			var receipt *types.Receipt
			var err error
			switch tc.action {
			case "get":
				v, e := p.Get(context.Background(), projectRef(), projects.GetOptions{}, types.CallOptions{})
				err = e
				receipt = v.Receipt
				if v.Data.Ref != nil {
					t.Error("failed read exposed data")
				}
			case "delete":
				v, e := p.Delete(context.Background(), projectRef(), types.CallOptions{})
				err = e
				receipt = v.Receipt
			case "create":
				v, e := p.Create(context.Background(), projects.CreateRequest{Name: "name"}, types.CallOptions{})
				err = e
				receipt = v.Receipt
			case "update":
				v, e := p.Update(context.Background(), projects.UpdateRequest{Ref: projectRef(), Name: managementPtr("name")}, types.CallOptions{})
				err = e
				receipt = v.Receipt
			}
			if types.ErrorCode(err) != tc.code || calls != 1 || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("project error contract broken", err)
			}
			if tc.outcome != "" && (receipt == nil || receipt.Outcome != tc.outcome || (len(receipt.ResourceRefs) > 0) != tc.knownID) {
				t.Fatal("project receipt evidence lost")
			}
		})
	}
}

const v1RegistryBody = `{"id":"r/one","name":"registry","status":"ACTIVE","createdAt":"2025-01-01T03:00:00+03:00","size":900719925474099312345,"sizeLimit":900719925474099312346,"used":0.12345678901234567890123456789,"ref":"ignored-provider-field"}`

func TestRegistryV1LifecyclePreservesItsSchema(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "cr.selcloud.ru" {
			t.Error("incorrect registry endpoint")
		}
		if r.Method == "POST" {
			if r.URL.Path != "/api/v1/registries" {
				t.Error("registry v1 create path incorrect")
			}
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "registry" {
				t.Error("registry create fields lost")
			}
			return inventoryReply(201, v1RegistryBody), nil
		}
		if r.URL.EscapedPath() != "/api/v1/registries/r%2Fone" {
			t.Error("registry ID not escaped")
		}
		if r.Method == "GET" {
			return inventoryReply(200, v1RegistryBody), nil
		}
		return inventoryReply(204, ""), nil
	}, 0)
	r := projectView(t, c).Registry().V1.Registries
	created, err := r.Create(context.Background(), registry.CreateRequest{Name: "registry"}, types.CallOptions{})
	if err != nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref.APIGeneration != "v1" || created.Data.SizeLimit.String() != "900719925474099312346" || created.Data.Used.String() != "0.12345678901234567890123456789" || created.Data.CreatedAt.Location() != time.UTC {
		t.Fatal("registry v1 schema changed", err)
	}
	read, err := r.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if err != nil || read.Data.Ref.APIGeneration != "v1" || read.Receipt != nil {
		t.Fatal("registry get failed", err)
	}
	deleted, err := r.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if err != nil || deleted.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal("registry delete failed", err)
	}
}

func TestRegistryV1PaginationRequiresConfirmedEnd(t *testing.T) {
	offset, _ := types.ParseInteger("900719925474099312345")
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v1/registries" || !r.URL.Query().Has("search") || r.URL.Query().Get("sort_type") != "desc" {
			t.Error("registry v1 filters lost")
		}
		if calls == 1 {
			if r.URL.Query().Get("offset") != offset.String() {
				t.Error("initial offset rounded")
			}
			return inventoryReply(200, "["+v1RegistryBody+"]"), nil
		}
		if r.URL.Query().Get("offset") != "900719925474099312346" {
			t.Error("next offset rounded")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	result, err := projectView(t, c).Registry().V1.Registries.Collect(context.Background(), registry.V1ListOptions{Offset: &offset, Search: managementPtr(""), SortField: managementPtr("name"), SortType: managementPtr("desc")}, pagination.Options{PageSize: 10})
	if err != nil || !result.Complete || result.Pages != 2 || calls != 2 || len(result.Items) != 1 || result.Items[0].Ref.APIGeneration != "v1" {
		t.Fatal("short positive page mistaken for end", err)
	}
	for _, tc := range []struct {
		status int
		body   string
		code   types.Code
	}{
		{200, "", types.ProtocolError}, {200, "null", types.ProtocolError}, {200, "[]", ""}, {204, "", ""}, {204, "[]", types.ProtocolError},
	} {
		client := inventoryClient(t, func(*http.Request) (*http.Response, error) { return inventoryReply(tc.status, tc.body), nil }, 0)
		page, e := projectView(t, client).Registry().V1.Registries.ListPage(context.Background(), registry.V1ListOptions{}, pagination.Options{})
		if types.ErrorCode(e) != tc.code || (e == nil && !page.Done) {
			t.Fatal("empty registry page contract broken", tc.status, tc.body, e)
		}
	}
}

func TestNewMutationsNeverReplayLostResponses(t *testing.T) {
	for _, action := range []string{"project.create", "project.update", "project.delete", "registry.create", "registry.delete"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }, 0)
			p := c.Account().Projects()
			r := projectView(t, c).Registry().V1.Registries
			var receipt *types.Receipt
			var err error
			switch action {
			case "project.create":
				v, e := p.Create(context.Background(), projects.CreateRequest{Name: "name"}, types.CallOptions{})
				receipt = v.Receipt
				err = e
			case "project.update":
				v, e := p.Update(context.Background(), projects.UpdateRequest{Ref: projectRef()}, types.CallOptions{})
				receipt = v.Receipt
				err = e
			case "project.delete":
				v, e := p.Delete(context.Background(), projectRef(), types.CallOptions{})
				receipt = v.Receipt
				err = e
			case "registry.create":
				v, e := r.Create(context.Background(), registry.CreateRequest{Name: "name"}, types.CallOptions{})
				receipt = v.Receipt
				err = e
			case "registry.delete":
				v, e := r.Delete(context.Background(), registryRef(), types.CallOptions{})
				receipt = v.Receipt
				err = e
			}
			if types.ErrorCode(err) != types.UnknownOutcome || receipt == nil || receipt.Outcome != types.Unknown || receipt.Attempts != 1 || calls != 1 {
				t.Fatal("mutation replayed or receipt lost", err)
			}
		})
	}
}

func TestNewResourcePreflightRejectsWrongScopeAndGeneration(t *testing.T) {
	for _, kind := range []string{"project.name", "project.scope", "project.kind", "registry.generation", "registry.scope", "registry.kind"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; return inventoryReply(204, ""), nil }, 0)
			var receipt *types.Receipt
			var err error
			if strings.HasPrefix(kind, "project") {
				ref := projectRef()
				if kind == "project.name" {
					v, e := c.Account().Projects().Create(context.Background(), projects.CreateRequest{Name: strings.Repeat("п", 65)}, types.CallOptions{})
					receipt = v.Receipt
					err = e
				} else {
					if kind == "project.scope" {
						ref.Scope.AccountID = "foreign"
					}
					if kind == "project.kind" {
						ref.Kind = "server"
					}
					v, e := c.Account().Projects().Delete(context.Background(), ref, types.CallOptions{})
					receipt = v.Receipt
					err = e
				}
			} else {
				ref := registryRef()
				if kind == "registry.generation" {
					ref.APIGeneration = "v2"
				}
				if kind == "registry.scope" {
					ref.Scope.ProjectID = "foreign"
				}
				if kind == "registry.kind" {
					ref.Kind = "repository"
				}
				v, e := projectView(t, c).Registry().V1.Registries.Delete(context.Background(), ref, types.CallOptions{})
				receipt = v.Receipt
				err = e
			}
			if err == nil || receipt == nil || receipt.Outcome != types.NotSent || receipt.Attempts != 0 || calls != 0 {
				t.Fatal("invalid mutation reached resource transport", err)
			}
		})
	}
}

func TestRegistryV1DecodeAndDeleteErrorsPreserveEvidence(t *testing.T) {
	for _, tc := range []struct {
		action  string
		status  int
		body    string
		code    types.Code
		outcome types.Outcome
		hasRef  bool
	}{
		{"create", 201, `{"id":"known","createdAt":"malformed"}`, types.ProtocolError, types.Accepted, true},
		{"create", 201, `{"name":"missing-id"}`, types.ProtocolError, types.Accepted, false},
		{"create", 409, "", types.Conflict, types.Rejected, false},
		{"get", 200, `{"id":"foreign"}`, types.ScopeMismatch, "", false},
		{"get", 404, "", types.NotFound, "", false},
		{"delete", 404, "", types.UnknownOutcome, types.Unknown, true},
	} {
		c := inventoryClient(t, func(*http.Request) (*http.Response, error) { return inventoryReply(tc.status, tc.body), nil }, 0)
		r := projectView(t, c).Registry().V1.Registries
		var receipt *types.Receipt
		var err error
		switch tc.action {
		case "create":
			v, e := r.Create(context.Background(), registry.CreateRequest{Name: "name"}, types.CallOptions{})
			receipt = v.Receipt
			err = e
		case "get":
			v, e := r.Get(context.Background(), registryRef(), types.CallOptions{})
			receipt = v.Receipt
			err = e
			if v.Data.Ref != nil {
				t.Error("failed registry read exposed data")
			}
		case "delete":
			v, e := r.Delete(context.Background(), registryRef(), types.CallOptions{})
			receipt = v.Receipt
			err = e
		}
		if types.ErrorCode(err) != tc.code {
			t.Fatal("registry error mapping broken", tc.action, err)
		}
		if tc.outcome != "" && (receipt == nil || receipt.Outcome != tc.outcome || (len(receipt.ResourceRefs) > 0) != tc.hasRef) {
			t.Fatal("registry receipt evidence lost")
		}
	}
}
