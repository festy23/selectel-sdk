// Package vmware implements source-bound Cloud Director operations. It never
// interprets vendor catalog size as Selectel deployment support.
package vmware

import (
	"context"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(runtime *core.Runtime, scope types.Scope) *Client {
	return &Client{runtime: runtime, scope: scope}
}

func (c *Client) Versions(ctx context.Context, options types.CallOptions) (types.Result[SupportedVersions], error) {
	op := core.Operation{ID: "vmware.v36_3.versions.list", Service: "vmware", APIGeneration: "v36_3", Method: http.MethodGet, Effect: "read", Auth: types.ProtocolSpecific, RequireProject: true, Success: []int{200}, Headers: http.Header{"Accept": {"text/xml"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/versions", Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if c.scope.Region == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[SupportedVersions], error) {
		var v SupportedVersions
		if validateVersionsXML(raw) != nil || xml.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.VersionInfo == nil {
			return core.Decoded[SupportedVersions]{}, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for _, info := range v.VersionInfo {
			if info.Version == nil || *info.Version == "" || seen[*info.Version] {
				return core.Decoded[SupportedVersions]{}, types.NewError(types.ProtocolError)
			}
			seen[*info.Version] = true
		}
		return core.Decoded[SupportedVersions]{Data: v}, nil
	})
}
