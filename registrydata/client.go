// Package registrydata implements source-bound OCI content transfer separately from management.
package registrydata

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"regexp"
)

type Client struct {
	runtime   *core.Runtime
	scope     types.Scope
	Manifests *Manifests
	Blobs     *Blobs
	Uploads   *Uploads
	Tags      *Tags
}
type Manifests struct{ client *Client }

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Manifests = &Manifests{c}
	c.Blobs = &Blobs{c}
	c.Uploads = &Uploads{c}
	c.Tags = &Tags{c}
	return c
}

var repoPattern = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*$`)
var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

func (c *Client) operation(id string) (core.Operation, error) {
	definition, ok := ociOperations[id]
	if !ok {
		return core.Operation{}, types.NewError(types.CapabilityUnavailable)
	}
	return c.runtime.BindDocumented(definition, c.scope)
}
func (c *Client) Repository(registry, repository string) (types.ResourceRef, error) {
	if !repoPattern.MatchString(registry) || !repoPattern.MatchString(repository) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("registry_data.oci_v1.manifests.get")
	if e != nil {
		return types.ResourceRef{}, e
	}
	parent := types.ResourceRef{Scope: c.scope, Service: "registry_data", APIGeneration: "oci_v1", Kind: "registry_namespace", ID: registry, EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}
	return child(parent, "repository", repository), nil
}
func child(parent types.ResourceRef, kind, id string) types.ResourceRef {
	raw, _ := json.Marshal(parent)
	var saved types.ResourceRef
	_ = json.Unmarshal(raw, &saved)
	return types.ResourceRef{Scope: parent.Scope, Service: parent.Service, APIGeneration: parent.APIGeneration, Kind: kind, ID: id, Parent: &saved, EndpointID: parent.EndpointID, EndpointURL: parent.EndpointURL}
}
func (c *Client) valid(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != c.scope || ref.Service != "registry_data" || ref.APIGeneration != "oci_v1" {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != kind {
		return types.NewError(types.InvalidArgument)
	}
	if kind == "registry_namespace" {
		if ref.Parent != nil || !repoPattern.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	parentKind := "repository"
	if kind == "repository" {
		parentKind = "registry_namespace"
		if !repoPattern.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
	}
	if ref.Parent == nil || ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return c.valid(*ref.Parent, parentKind)
}
func (c *Manifests) Tag(repo types.ResourceRef, tag string) (types.ResourceRef, error) {
	if e := c.client.valid(repo, "repository"); e != nil {
		return types.ResourceRef{}, e
	}
	if !tagPattern.MatchString(tag) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	return child(repo, "manifest_tag", tag), nil
}

func (c *Manifests) Digest(repo types.ResourceRef, digest Digest) (types.ResourceRef, error) {
	if e := c.client.valid(repo, "repository"); e != nil {
		return types.ResourceRef{}, e
	}
	if _, e := ParseDigest(digest.String()); e != nil {
		return types.ResourceRef{}, e
	}
	return child(repo, "manifest_digest", digest.String()), nil
}
