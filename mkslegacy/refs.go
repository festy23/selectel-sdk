package mkslegacy

import (
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

func cloneRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		p := cloneRef(*ref.Parent)
		ref.Parent = &p
	}
	return ref
}

func (c *Client) validateNested(ref types.ResourceRef, kind string, op core.Operation) error {
	if kind == "cluster" {
		return c.validateRoot(ref, "cluster", op)
	}
	if ref.Validate() != nil || ref.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	parentKind := "cluster"
	if kind == "node" {
		parentKind = "nodegroup"
	}
	if ref.Kind != kind || ref.Service != "mks" || ref.APIGeneration != "v1" || ref.Scope != c.scope || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint || ref.Parent.Kind != parentKind {
		return types.NewError(types.ScopeMismatch)
	}
	return c.validateNested(*ref.Parent, parentKind, op)
}
func resourcePath(ref types.ResourceRef) string { return resourcePathDepth(ref, 0) }
func resourcePathDepth(ref types.ResourceRef, depth int) string {
	if depth > 16 {
		return "/invalid-reference"
	}
	switch ref.Kind {
	case "cluster":
		return "/clusters/" + segment(ref.ID)
	case "nodegroup":
		if ref.Parent != nil {
			return resourcePathDepth(*ref.Parent, depth+1) + "/nodegroups/" + segment(ref.ID)
		}
	case "node":
		if ref.Parent != nil {
			return resourcePathDepth(*ref.Parent, depth+1) + "/" + segment(ref.ID)
		}
	case "task":
		if ref.Parent != nil {
			return resourcePathDepth(*ref.Parent, depth+1) + "/tasks/" + segment(ref.ID)
		}
	}
	return "/invalid-reference"
}
func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
