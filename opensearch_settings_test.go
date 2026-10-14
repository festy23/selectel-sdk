package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/databases/opensearchsettings"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

func osSettingsClient(t *testing.T, wire transportFunc) *sdk.Client {
	t.Helper()
	scope := osRef().Scope
	c, e := sdk.New(sdk.Config{AccountID: scope.AccountID, Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Mode != types.Basic || r.Service != "opensearch_data" || r.Scope != scope {
			t.Fatal("wrong credential request", r.Mode, r.Service)
		}
		return credentials.Material{Identity: "engine-user", Revision: "one", Scheme: credentials.Basic, Binding: scope, Audience: "selectel", Username: types.Protect([]byte("engine-admin")), Password: types.Protect([]byte("synthetic-private-engine-password"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "engine-admin" || p != "synthetic-private-engine-password" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatal("engine Basic auth boundary lost")
		}
		return wire(r)
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "opensearch_data", APIGeneration: "opensearch_2_18", Scope: scope}: {ID: "exact-search-cluster", URL: "https://10.0.0.10:9200", APIGeneration: "opensearch_2_18", Scope: scope, Audience: "selectel"}}})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestOpenSearchSettingsReadUsesExplicitEngineEndpointAndBasicAuth(t *testing.T) {
	yes := true
	c := osSettingsClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "10.0.0.10:9200" || r.URL.Path != "/_cluster/settings" || r.URL.Query().Get("include_defaults") != "true" {
			t.Fatal("wrong settings endpoint", r.URL)
		}
		return inventoryReply(200, `{"persistent":{"action.auto_create_index":"synthetic-private-pattern"},"transient":{},"defaults":{"cluster.max_shards_per_node":"1000"}}`), nil
	})
	result, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Get(context.Background(), opensearchsettings.GetOptions{IncludeDefaults: &yes})
	if e != nil || result.Data.Persistent == nil || !strings.Contains(string(result.Data.Persistent.Reveal()), "synthetic-private-pattern") {
		t.Fatal("protected read mapping", e)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "synthetic-private-pattern") {
		t.Fatal("settings disclosed")
	}
}
func osSetting(raw string) *types.Nullable[types.ProtectedJSON] {
	return types.Value(types.ProtectedJSON{ProtectedValue: types.Protect([]byte(raw))})
}
func TestOpenSearchSettingsUpdatePreservesPersistentTransientFalseExactNumberAndReset(t *testing.T) {
	c := osSettingsClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/_cluster/settings" {
			t.Fatal("update binding", r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"persistent":{"action.auto_create_index":false,"cluster.max_shards_per_node":900719925474099312345},"transient":{"action.search.shard_count.limit":null}}` {
			t.Fatal("update/reset mapping changed")
		}
		return inventoryReply(200, `{"acknowledged":false,"persistent":{"cluster.max_shards_per_node":"900719925474099312345"},"transient":{}}`), nil
	})
	request := opensearchsettings.UpdateRequest{Persistent: &opensearchsettings.SettingsValues{ActionAutoCreateIndex: osSetting("false"), ClusterMaxShardsPerNode: osSetting("900719925474099312345")}, Transient: &opensearchsettings.SettingsValues{ActionSearchShardCountLimit: types.Null[types.ProtectedJSON]()}}
	result, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Update(context.Background(), request, opensearchsettings.UpdateOptions{})
	if e != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || result.Data.Acknowledged == nil || *result.Data.Acknowledged {
		t.Fatal("update acknowledgement/receipt changed", e)
	}
}
func TestOpenSearchSettingsProtectedFormatsAndScalarValidation(t *testing.T) {
	c := osSettingsClient(t, func(r *http.Request) (*http.Response, error) { t.Fatal("invalid value dispatched"); return nil, nil })
	request := opensearchsettings.UpdateRequest{Persistent: &opensearchsettings.SettingsValues{ActionAutoCreateIndex: osSetting(`"synthetic-private-setting"`)}}
	paths := []any{request, request.Persistent, request.Persistent.ActionAutoCreateIndex, *request.Persistent.ActionAutoCreateIndex}
	for _, path := range paths {
		for _, verb := range []string{"%+v", "%#v"} {
			formatted := fmt.Sprintf(verb, path)
			if strings.Contains(formatted, "synthetic-private-setting") || strings.Contains(formatted, "115 121 110 116") || strings.Contains(formatted, "0x73, 0x79, 0x6e") {
				t.Fatal("encoded protected value disclosed")
			}
		}
		raw, _ := json.Marshal(path)
		if strings.Contains(string(raw), "synthetic-private-setting") {
			t.Fatal("JSON disclosed")
		}
	}
	text, e := request.Persistent.ActionAutoCreateIndex.MarshalText()
	if e != nil || strings.Contains(string(text), "synthetic-private-setting") {
		t.Fatal("text disclosed")
	}
	value, present := request.Persistent.ActionAutoCreateIndex.Value()
	if !present || string(value.Reveal()) != `"synthetic-private-setting"` {
		t.Fatal("authorized reveal lost")
	}
	for _, bad := range []string{`{"unsupported":"injection"}`, `[1]`, `invalid`} {
		request.Persistent.ActionAutoCreateIndex = osSetting(bad)
		_, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Update(context.Background(), request, opensearchsettings.UpdateOptions{})
		if types.ErrorCode(e) != types.InvalidArgument {
			t.Fatal("non-scalar accepted", e)
		}
	}
	_, e = regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Update(context.Background(), opensearchsettings.UpdateRequest{}, opensearchsettings.UpdateOptions{})
	if types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal("no settings accepted", e)
	}
}
func TestOpenSearchSettingsUnknownAndMalformedResponsesRetainReceipt(t *testing.T) {
	for _, scenario := range []string{"lost", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := osSettingsClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if scenario == "lost" {
					return nil, io.ErrUnexpectedEOF
				}
				return inventoryReply(200, `{"persistent":{},"transient":{}}`), nil
			})
			request := opensearchsettings.UpdateRequest{Persistent: &opensearchsettings.SettingsValues{ActionAutoCreateIndex: osSetting("false")}}
			result, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Update(context.Background(), request, opensearchsettings.UpdateOptions{})
			if calls != 1 || result.Receipt == nil || result.Receipt.Endpoint.APIGeneration != "opensearch_2_18" {
				t.Fatal("lost receipt/replay", e)
			}
			if scenario == "lost" {
				if types.ErrorCode(e) != types.UnknownOutcome || result.Receipt.Outcome != types.Unknown {
					t.Fatal(e)
				}
			} else {
				if types.ErrorCode(e) != types.ProtocolError || result.Receipt.Outcome != types.Accepted {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestOpenSearchSettingsRequiresExplicitEndpointAndRejectsIAMCredentials(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("missing engine endpoint used IAM/DBaaS fallback")
		return nil, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	_, e = regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Get(context.Background(), opensearchsettings.GetOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable {
		t.Fatal("endpoint guessed", e)
	}
	scope := osRef().Scope
	c, e = sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("IAM credentials forwarded to cluster")
		return nil, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "opensearch_data", APIGeneration: "opensearch_2_18", Scope: scope}: {ID: "exact-cluster", URL: "https://10.0.0.10:9200", APIGeneration: "opensearch_2_18", Scope: scope, Audience: "selectel"}}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Get(context.Background(), opensearchsettings.GetOptions{})
	if types.ErrorCode(e) != types.CredentialsUnavailable {
		t.Fatal("IAM scheme accepted", e)
	}
}
func TestOpenSearchSettingsAllTwelveAllowedFieldsAndOptionalQueries(t *testing.T) {
	values := opensearchsettings.SettingsValues{ActionAutoCreateIndex: osSetting("false"), ActionSearchShardCountLimit: osSetting("1000"), IndicesBreakerFielddataLimit: osSetting(`"40%"`), IndicesBreakerRequestLimit: osSetting(`"50%"`), IndicesBreakerTotalLimit: osSetting(`"95%"`), ClusterMaxShardsPerNode: osSetting("1000"), ClusterSearchRequestSlowlogLevel: osSetting(`"WARN"`), ClusterSearchRequestSlowlogThresholdWarn: osSetting(`"10s"`), ClusterSearchRequestSlowlogThresholdInfo: osSetting(`"5s"`), ClusterSearchRequestSlowlogThresholdDebug: osSetting(`"2s"`), ClusterSearchRequestSlowlogThresholdTrace: osSetting(`"500ms"`), SearchPhaseTookEnabled: osSetting("true")}
	no := false
	manager := "1m"
	timeout := "45s"
	c := osSettingsClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("flat_settings") != "false" || r.URL.Query().Get("cluster_manager_timeout") != "1m" || r.URL.Query().Get("timeout") != "45s" || r.URL.Query().Has("include_defaults") {
			t.Fatal("query mapping changed")
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil || len(body["persistent"]) != 12 {
			t.Fatal("not twelve settings")
		}
		for _, key := range []string{"action.auto_create_index", "action.search.shard_count.limit", "indices.breaker.fielddata.limit", "indices.breaker.request.limit", "indices.breaker.total.limit", "cluster.max_shards_per_node", "cluster.search.request.slowlog.level", "cluster.search.request.slowlog.threshold.warn", "cluster.search.request.slowlog.threshold.info", "cluster.search.request.slowlog.threshold.debug", "cluster.search.request.slowlog.threshold.trace", "search.phase_took_enabled"} {
			if _, ok := body["persistent"][key]; !ok {
				t.Fatal("missing Selectel field", key)
			}
		}
		return inventoryReply(200, `{"acknowledged":true,"persistent":{},"transient":{}}`), nil
	})
	_, e := regionalView(t, c, "ru-2").Databases().V2.OpenSearch.Settings.Update(context.Background(), opensearchsettings.UpdateRequest{Persistent: &values}, opensearchsettings.UpdateOptions{FlatSettings: &no, ClusterManagerTimeout: &manager, Timeout: &timeout})
	if e != nil {
		t.Fatal(e)
	}
}
func TestOpenSearchSettingsCatalogSeparatesProtocolGenerationAndBasicAuth(t *testing.T) {
	for _, action := range []string{"get", "update"} {
		capability, e := catalog.Default().Describe("opensearch_data.opensearch_2_18.cluster_settings." + action)
		if e != nil || capability.Implementation.Status != "implemented" || capability.Auth.Scheme != types.Basic || capability.APIGeneration != "opensearch_2_18" || len(capability.WaitGoals) != 0 {
			t.Fatal("protocol binding unavailable/overclaims", action, e)
		}
	}
}
