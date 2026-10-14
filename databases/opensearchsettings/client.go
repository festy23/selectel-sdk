// Package opensearchsettings implements the Selectel-supported OpenSearch 2.18
// cluster settings protocol, using explicitly configured cluster credentials.
package opensearchsettings

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }
func (c *Client) operation(action, method string) (core.Operation, error) {
	op, e := c.runtime.BindDocumented(core.Operation{ID: "opensearch_data.opensearch_2_18.cluster_settings." + action, Service: "opensearch_data", APIGeneration: "opensearch_2_18", Auth: types.Basic, RequireProject: true, RequireLocation: true, Method: method, Effect: map[bool]string{true: "read", false: "mutation"}[method == "GET"], Success: []int{200}}, c.scope)
	if e != nil {
		return op, e
	}
	parsed, e := url.Parse(op.FixedEndpoint)
	if e != nil || (parsed.Path != "" && parsed.Path != "/") {
		return op, types.NewError(types.InvalidArgument)
	}
	return op, nil
}
func query(flat *bool, manager *string) url.Values {
	q := url.Values{}
	if flat != nil {
		q.Set("flat_settings", strconv.FormatBool(*flat))
	}
	if manager != nil {
		q.Set("cluster_manager_timeout", *manager)
	}
	return q
}
func object(v *types.ProtectedJSON) bool {
	if v == nil {
		return false
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(v.Reveal(), &value) == nil && value != nil
}
func (c *Client) Get(ctx context.Context, o GetOptions) (types.Result[GetResponse], error) {
	op, e := c.operation("get", "GET")
	q := query(o.FlatSettings, o.ClusterManagerTimeout)
	if o.IncludeDefaults != nil {
		q.Set("include_defaults", strconv.FormatBool(*o.IncludeDefaults))
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/_cluster/settings", Query: q, Validate: func() error { return e }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetResponse], error) {
		var value GetResponse
		if json.Unmarshal(raw, &value) != nil || value.Validate() != nil || !object(value.Persistent) || !object(value.Transient) || (value.Defaults != nil && !object(value.Defaults)) {
			return core.Decoded[GetResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetResponse]{Data: value}, nil
	})
}

// updateBody is the sole disclosure boundary for protected input settings.
// The generated closed DTO supplies the Selectel allowlist; no arbitrary keys or
// wildcard patterns can enter this map.
func updateBody(request UpdateRequest) ([]byte, error) {
	payload := map[string]map[string]json.RawMessage{}
	count := 0
	for _, section := range []struct {
		name   string
		values *SettingsValues
	}{{"persistent", request.Persistent}, {"transient", request.Transient}} {
		if section.values == nil {
			continue
		}
		out := map[string]json.RawMessage{}
		v := reflect.ValueOf(*section.values)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.IsNil() {
				continue
			}
			setting := field.Interface().(*types.Nullable[types.ProtectedJSON])
			value, present := setting.Value()
			raw := []byte("null")
			if present {
				raw = bytes.TrimSpace(value.Reveal())
				if !utf8.Valid(raw) || !json.Valid(raw) || len(raw) == 0 || raw[0] == '{' || raw[0] == '[' {
					return nil, types.NewError(types.InvalidArgument)
				}
			}
			key := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			out[key] = json.RawMessage(raw)
			count++
		}
		payload[section.name] = out
	}
	if count == 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	body, e := json.Marshal(payload)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return body, nil
}
func (c *Client) Update(ctx context.Context, request UpdateRequest, o UpdateOptions) (types.Result[UpdateResponse], error) {
	op, binding := c.operation("update", "PUT")
	body, encoding := updateBody(request)
	q := query(o.FlatSettings, o.ClusterManagerTimeout)
	if o.Timeout != nil {
		q.Set("timeout", *o.Timeout)
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/_cluster/settings", Body: body, Query: q, Validate: func() error {
		if encoding != nil {
			return encoding
		}
		return binding
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UpdateResponse], error) {
		var value UpdateResponse
		if json.Unmarshal(raw, &value) != nil || value.Validate() != nil || !object(value.Persistent) || !object(value.Transient) {
			return core.Decoded[UpdateResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[UpdateResponse]{Data: value}, nil
	})
}
