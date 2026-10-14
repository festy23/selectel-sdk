package selectelsdk_test

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"strings"
	"testing"
)

func TestDatabaseTypesCanBeSelectedThroughScopedAPI(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/datastore-types" || r.URL.Host != "ru-1.dbaas.selcloud.ru" || len(r.URL.Query()) != 0 {
			t.Fatal("unexpected reference request", r.URL)
		}
		return inventoryReply(200, `{"datastore-types":[{"id":"postgres-16","engine":"postgresql","version":"16"}]}`), nil
	}, 0)
	result, err := regionalView(t, c, "ru-1").Databases().V1.DatastoreTypes.Collect(context.Background(), pagination.Options{})
	if err != nil || !result.Complete || len(result.Items) != 1 || *result.Items[0].Engine != "postgresql" || *result.Items[0].Version != "16" {
		t.Fatal(result, err)
	}
}

func TestDatabaseFlavorFiltersAndExactReadPreserveCapacity(t *testing.T) {
	name := ""
	size := "standard"
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/flavors":
			if !r.URL.Query().Has("name") || r.URL.Query().Get("fl_size") != "standard" {
				t.Fatal("filters lost", r.URL)
			}
			return inventoryReply(200, `{"flavors":[{"id":"flavor","ram":900719925474099312345,"disk":20,"vcpus":2,"subtype":"CPU","host":{"available_count":0}}]}`), nil
		case "/v1/flavors/flavor":
			return inventoryReply(200, `{"flavor":{"id":"flavor","ram":900719925474099312345}}`), nil
		default:
			t.Fatal(r.URL)
			return nil, nil
		}
	}, 0)
	flavors := regionalView(t, c, "ru-1").Databases().V1.Flavors
	page, err := flavors.Collect(context.Background(), databases.FlavorListOptions{Name: &name, FlSize: &size}, pagination.Options{})
	if err != nil || page.Items[0].RAM.String() != "900719925474099312345" || *page.Items[0].Subtype != "CPU" {
		t.Fatal(page, err)
	}
	got, err := flavors.Get(context.Background(), "flavor")
	if err != nil || got.Data.RAM.String() != "900719925474099312345" {
		t.Fatal(got, err)
	}
}

func TestDatabaseParameterConstraintsRetainPrimitiveKindsAndPrecision(t *testing.T) {
	ram := "900719925474099312345"
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/configuration-parameters" || r.URL.Query().Get("ram") != ram {
			t.Fatal("parameter request", r.URL)
		}
		return inventoryReply(200, `{"configuration-parameters":[{"id":"parameter","name":"limit","type":"integer","unit":"bytes","min":null,"max":900719925474099312345,"default_value":false,"choices":[null,"unlimited",0,true],"invalid_values":[],"factor":10,"is_changeable":false}]}`), nil
	}, 0)
	parameters := regionalView(t, c, "ru-1").Databases().V1.ConfigurationParameters
	result, err := parameters.Collect(context.Background(), databases.ConfigurationParameterListOptions{RAM: &ram}, pagination.Options{})
	if err != nil || len(result.Items) != 1 {
		t.Fatal(result, err)
	}
	v := result.Items[0]
	if v.Min.String() != "null" || v.Max.String() != "900719925474099312345" || v.DefaultValue.String() != "false" || v.Choices[1].String() != `"unlimited"` || *v.Unit != "bytes" || v.Factor.String() != "10" {
		t.Fatal("parameter constraints changed", v)
	}
}

func TestDatabaseUserSettingsUseSDKDocumentedSeparateReference(t *testing.T) {
	datastore := "db-id"
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/user-setting-parameters" || r.URL.Query().Get("datastore_id") != datastore {
			t.Fatal(r.URL)
		}
		return inventoryReply(200, `{"user-setting-parameters":[{"id":"20d7bcf4-f8d6-4bf6-b8f6-46cb440a87f4","name":"statement_timeout","type":"int","min":0,"max":900719925474099312345,"default_value":"0","unit":"ms","apply_mechanism":"guc","choices":null,"is_available_for_customer":true}]}`), nil
	}, 0)
	result, err := regionalView(t, c, "ru-1").Databases().V1.UserSettingParameters.Collect(context.Background(), databases.UserSettingParameterListOptions{DatastoreID: &datastore}, pagination.Options{})
	if err != nil || result.Items[0].Max == nil || *result.Items[0].ApplyMechanism != "guc" {
		t.Fatal(result, err)
	}
	maximum, present := result.Items[0].Max.Value()
	if !present || string(maximum.Reveal()) != "900719925474099312345" {
		t.Fatal("protected constraint precision lost")
	}
}

func TestDatabaseReferenceReadsRejectForeignIDAndBadEnvelopes(t *testing.T) {
	foreign := true
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if foreign {
			return inventoryReply(200, `{"datastore-type":{"id":"other"}}`), nil
		}
		return inventoryReply(200, `{"datastore-type":null}`), nil
	}, 0)
	service := regionalView(t, c, "ru-1").Databases().V1.DatastoreTypes
	_, err := service.Get(context.Background(), "expected")
	if types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatal("foreign reference accepted", err)
	}
	foreign = false
	_, err = service.Get(context.Background(), "expected")
	if types.ErrorCode(err) != types.ProtocolError {
		t.Fatal("missing reference accepted", err)
	}
}

func TestDatabaseReferenceCollectionsBoundAndBindContinuation(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"flavors":[{"id":"one"},{"id":"two"}]}`), nil
	}, 0)
	service := regionalView(t, c, "ru-1").Databases().V1.Flavors
	first, err := service.Collect(context.Background(), databases.FlavorListOptions{}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || first.Complete || first.NextCursor == nil || len(first.Items) != 1 {
		t.Fatal(first, err)
	}
	second, err := service.Collect(context.Background(), databases.FlavorListOptions{}, pagination.Options{Cursor: first.NextCursor})
	if err != nil || !second.Complete || len(second.Items) != 1 || *second.Items[0].ID != "two" {
		t.Fatal(second, err)
	}
	name := "changed"
	_, err = service.Collect(context.Background(), databases.FlavorListOptions{Name: &name}, pagination.Options{Cursor: first.NextCursor})
	if types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatal("foreign filters accepted", err)
	}
}

func TestDatabaseAllReferenceGetMappingsAndErrors(t *testing.T) {
	for _, family := range []string{"datastore-type", "flavor", "configuration-parameter", "user-setting-parameter"} {
		t.Run(family, func(t *testing.T) {
			status := 200
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/"+family+"s/20d7bcf4-f8d6-4bf6-b8f6-46cb440a87f4" {
					t.Fatal("exact reference path", r.URL)
				}
				if status != 200 {
					return inventoryReply(status, `{"error":{"message":"synthetic-private-provider-body"}}`), nil
				}
				return inventoryReply(200, `{"`+family+`":{"id":"20d7bcf4-f8d6-4bf6-b8f6-46cb440a87f4","name":"setting","engine":"postgresql","version":"16"}}`), nil
			}, 0)
			v := regionalView(t, c, "ru-1").Databases().V1
			invoke := func() error {
				const id = "20d7bcf4-f8d6-4bf6-b8f6-46cb440a87f4"
				switch family {
				case "datastore-type":
					result, e := v.DatastoreTypes.Get(context.Background(), id)
					if e == nil && *result.Data.Version != "16" {
						t.Fatal(result)
					}
					return e
				case "flavor":
					result, e := v.Flavors.Get(context.Background(), id)
					if e == nil && *result.Data.Name != "setting" {
						t.Fatal(result)
					}
					return e
				case "configuration-parameter":
					result, e := v.ConfigurationParameters.Get(context.Background(), id)
					if e == nil && *result.Data.Name != "setting" {
						t.Fatal(result)
					}
					return e
				default:
					result, e := v.UserSettingParameters.Get(context.Background(), id)
					if e == nil && *result.Data.Name != "setting" {
						t.Fatal(result)
					}
					return e
				}
			}
			if err := invoke(); err != nil {
				t.Fatal(err)
			}
			for code, want := range map[int]types.Code{400: types.ProviderError, 403: types.PermissionDenied, 404: types.NotFound} {
				status = code
				if err := invoke(); types.ErrorCode(err) != want {
					t.Fatal(code, err)
				}
			}
		})
	}
}

func TestDatabaseReferenceCatalogContainsEightCompleteBindings(t *testing.T) {
	for resource, service := range map[string]string{"datastore_types": "DatastoreTypes", "flavors": "Flavors", "configuration_parameters": "ConfigurationParameters", "user_setting_parameters": "UserSettingParameters"} {
		for action, method := range map[string]string{"list": "ListPage", "get": "Get"} {
			id := "databases.v1." + resource + "." + action
			capability, err := catalog.Default().Describe(id)
			if err != nil || capability.Implementation.Status != "implemented" || (capability.Verification.Level != "offline" && capability.Verification.Level != "none") || capability.GoMethod == nil || *capability.GoMethod != "databases."+service+"."+method {
				t.Fatal("missing complete offline binding", id, capability, err)
			}
		}
	}
}

func TestDatabaseUserSettingsPreserveOmittedNullAndProtectedValues(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"user-setting-parameters":[{"id":"omitted"},{"id":"null","min":null,"max":null,"choices":null},{"id":"values","min":0,"max":false,"choices":[null,"synthetic-private-choice"]},{"id":"empty","choices":[]}]}`), nil
	}, 0)
	result, err := regionalView(t, c, "ru-1").Databases().V1.UserSettingParameters.Collect(context.Background(), databases.UserSettingParameterListOptions{}, pagination.Options{})
	if err != nil || len(result.Items) != 4 {
		t.Fatal("reference result missing", err)
	}

	omitted, nulls, values := result.Items[0], result.Items[1], result.Items[2]
	if omitted.Min != nil || omitted.Max != nil || omitted.Choices != nil || nulls.Min == nil || nulls.Max == nil || nulls.Choices == nil {
		t.Fatal("field presence collapsed")
	}
	if _, valid := nulls.Min.Value(); valid {
		t.Fatal("null minimum became value")
	}
	if _, valid := nulls.Max.Value(); valid {
		t.Fatal("null maximum became value")
	}
	if _, valid := nulls.Choices.Value(); valid {
		t.Fatal("null choices became value")
	}
	minimum, valid := values.Min.Value()
	if !valid || string(minimum.Reveal()) != "0" {
		t.Fatal("zero minimum changed")
	}
	maximum, valid := values.Max.Value()
	if !valid || string(maximum.Reveal()) != "false" {
		t.Fatal("false maximum changed")
	}
	choices, valid := values.Choices.Value()
	if !valid || len(choices) != 2 || string(choices[0].Reveal()) != "null" || string(choices[1].Reveal()) != `"synthetic-private-choice"` {
		t.Fatal("explicit choices reveal changed")
	}

	raw, err := json.Marshal(result.Items)
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		t.Fatal("invalid safe JSON")
	}
	for _, field := range []string{"min", "max", "choices"} {
		if _, present := items[0][field]; present {
			t.Fatal("omitted field inserted", field)
		}
		if string(items[1][field]) != "null" {
			t.Fatal("explicit null collapsed", field, string(items[1][field]))
		}
		if _, present := items[2][field]; !present {
			t.Fatal("value field omitted", field)
		}
	}
	if string(items[3]["choices"]) != "[]" {
		t.Fatal("empty choices collapsed")
	}
	if strings.Contains(string(raw), "synthetic-private-choice") || strings.Contains(fmt.Sprintf("%+v", result), "synthetic-private-choice") {
		t.Fatal("protected setting value disclosed")
	}
}

func TestDatabaseNullableProtectedFieldsNeverFormatPayloadBytes(t *testing.T) {
	const marker = "synthetic-private-bound"
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"user-setting-parameters":[{"min":"synthetic-private-bound","max":"synthetic-private-bound","choices":["synthetic-private-bound"]}]}`), nil
	}, 0)
	result, err := regionalView(t, c, "ru-1").Databases().V1.UserSettingParameters.Collect(context.Background(), databases.UserSettingParameterListOptions{}, pagination.Options{})
	if err != nil || len(result.Items) != 1 {
		t.Fatal("reference missing", err)
	}
	value := result.Items[0]
	decimal := strings.Trim(fmt.Sprintf("%v", []byte(marker)), "[]")
	hexParts := make([]string, len(marker))
	for i, b := range []byte(marker) {
		hexParts[i] = fmt.Sprintf("0x%x", b)
	}
	hex := strings.Join(hexParts, ", ")
	paths := map[string]any{"min-pointer": value.Min, "min-value": *value.Min, "max-pointer": value.Max, "max-value": *value.Max, "choices-pointer": value.Choices, "choices-value": *value.Choices, "nested-value": value, "nested-pointer": &value, "collection": result}
	for name, path := range paths {
		for _, verb := range []string{"%+v", "%#v"} {
			output := fmt.Sprintf(verb, path)
			if strings.Contains(output, marker) || strings.Contains(output, decimal) || strings.Contains(output, hex) {
				t.Fatalf("%s %s exposed protected payload bytes", name, verb)
			}
		}
	}

	for name, path := range paths {
		output, err := json.Marshal(path)
		if err != nil || strings.Contains(string(output), marker) || strings.Contains(string(output), decimal) || strings.Contains(string(output), hex) {
			t.Fatalf("%s JSON protection failed", name)
		}
	}
	for name, path := range map[string]encoding.TextMarshaler{"min-pointer": value.Min, "min-value": *value.Min, "max-pointer": value.Max, "max-value": *value.Max, "choices-pointer": value.Choices, "choices-value": *value.Choices} {
		output, err := path.MarshalText()
		if err != nil || strings.Contains(string(output), marker) || strings.Contains(string(output), decimal) || strings.Contains(string(output), hex) || !strings.Contains(string(output), "[REDACTED]") {
			t.Fatalf("%s text protection failed", name)
		}
	}

	minimum, present := value.Min.Value()
	if !present || string(minimum.Reveal()) != `"synthetic-private-bound"` {
		t.Fatal("authorized reveal lost")
	}
}
