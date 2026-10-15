package registrydata

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"mime"
	"reflect"
	"strconv"
)

// FallbackTag derives the explicit OCI referrers-tag schema address. The source
// limits the encoded digest component to64 characters; this is not unique
// identity proof for algorithms with longer encodings. No lookup is performed.
func (c *Referrers) FallbackTag(subject types.ResourceRef) (types.ResourceRef, error) {
	if e := c.client.Manifests.valid(subject); e != nil {
		return types.ResourceRef{}, e
	}
	if subject.Kind != "manifest_digest" {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	digest, e := ParseDigest(subject.ID)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return c.client.Manifests.Tag(*subject.Parent, digest.Algorithm+"-"+digest.Encoded[:min(64, len(digest.Encoded))])
}

// PrepareTagIndex creates a protected source-only payload, never a cloud write.
// The caller explicitly selects/obtains current and separately calls Put.
func (c *Referrers) PrepareTagIndex(subject types.ResourceRef, current *ManifestDocument, content types.ResourceRef, input ManifestWriteOptions, change TagIndexChange, options TagIndexOptions) (PreparedTagIndex, error) {

	tag, e := c.FallbackTag(subject)
	if e != nil {
		return PreparedTagIndex{}, e
	}
	if change != TagIndexAdd && change != TagIndexRemove {
		return PreparedTagIndex{}, types.NewError(types.InvalidArgument)
	}
	if e := c.client.Manifests.valid(content); e != nil {
		return PreparedTagIndex{}, e
	}
	if content.Kind != "manifest_digest" {
		return PreparedTagIndex{}, types.NewError(types.InvalidArgument)
	}
	if !reflect.DeepEqual(content.Parent, subject.Parent) {
		return PreparedTagIndex{}, types.NewError(types.ScopeMismatch)
	}
	var descriptor json.RawMessage
	if change == TagIndexAdd {
		descriptor, e = c.tagDescriptor(subject, content, input)
		if e != nil {
			return PreparedTagIndex{}, e
		}
	}
	plan, e := c.client.runtime.PagePlan("registry_data.oci_v1.referrers.list", "oci_v1", c.client.scope, tag.EndpointID, subject)
	if e != nil {
		return PreparedTagIndex{}, e
	}
	if options.MaxDescriptors == 0 {
		options.MaxDescriptors = plan.Defaults.MaxItems
	}
	fields, rows, base, e := c.tagSnapshot(current, tag)
	if e != nil {
		return PreparedTagIndex{}, e
	}
	if uint(len(rows)) > options.MaxDescriptors {
		return PreparedTagIndex{}, types.NewError(types.ResponseLimit)
	}
	next := []json.RawMessage{}
	found := false
	for _, row := range rows {
		entry, e := decodeReferrer(row, *subject.Parent)
		if e != nil {
			return PreparedTagIndex{}, types.NewError(types.InvalidArgument)
		}
		if entry.Digest.String() == content.ID {
			found = true
			if change == TagIndexRemove {
				continue
			}
		}
		next = append(next, row)
	}
	changed := false
	if change == TagIndexAdd && !found {
		if uint(len(next)) >= options.MaxDescriptors {
			return PreparedTagIndex{}, types.NewError(types.ResponseLimit)
		}
		next = append(next, descriptor)
		changed = true
	}
	if change == TagIndexRemove && found {
		changed = true
	}
	out := PreparedTagIndex{Subject: child(*subject.Parent, "manifest_digest", subject.ID), Tag: tag, Changed: changed, DescriptorCount: uint(len(next)), BaseDigest: base}
	if !changed && current != nil {
		out.Write = ManifestWriteOptions{MediaType: OCIIndex, Payload: current.Payload}
		return out, nil
	}
	encodedRows, _ := json.Marshal(next)
	fields["manifests"] = encodedRows
	encoded, e := json.Marshal(fields)
	if e != nil {
		return PreparedTagIndex{}, types.NewError(types.InvalidArgument)
	}
	if uint64(len(encoded)) > c.client.runtime.StructuredLimit() {
		return PreparedTagIndex{}, types.NewError(types.ResponseLimit)
	}
	var protected types.ProtectedJSON
	_ = json.Unmarshal(encoded, &protected)
	out.Write = ManifestWriteOptions{MediaType: OCIIndex, Payload: protected}
	return out, nil
}
func (c *Referrers) tagSnapshot(current *ManifestDocument, tag types.ResourceRef) (map[string]json.RawMessage, []json.RawMessage, *Digest, error) {
	if current == nil {
		return map[string]json.RawMessage{"schemaVersion": json.RawMessage("2"), "mediaType": json.RawMessage(`"application/vnd.oci.image.index.v1+json"`)}, []json.RawMessage{}, nil, nil
	}
	if e := c.client.Manifests.valid(current.Address); e != nil {
		return nil, nil, nil, e
	}
	if !reflect.DeepEqual(current.Address, tag) {
		return nil, nil, nil, types.NewError(types.ScopeMismatch)
	}
	raw := current.Payload.Reveal()
	if uint64(len(raw)) > c.client.runtime.StructuredLimit() {
		return nil, nil, nil, types.NewError(types.ResponseLimit)
	}
	if current.MediaType != OCIIndex || current.Size.String() != strconv.Itoa(len(raw)) {
		return nil, nil, nil, types.NewError(types.InvalidArgument)
	}
	digest, e := ParseDigest(current.Digest.String())
	if e != nil {
		return nil, nil, nil, e
	}
	actual, e := hashBytes(raw, digest.Algorithm)
	if e != nil || actual != digest {
		return nil, nil, nil, types.NewError(types.InvalidArgument)
	}
	fields, e := nonnullObject(raw, []string{"schemaVersion", "mediaType", "manifests", "artifactType", "subject", "annotations"})
	if e != nil {
		return nil, nil, nil, types.NewError(types.InvalidArgument)
	}
	var version types.Integer
	if json.Unmarshal(fields["schemaVersion"], &version) != nil || version.String() != "2" {
		return nil, nil, nil, types.NewError(types.InvalidArgument)
	}
	if value, ok := fields["mediaType"]; ok {
		var media string
		if json.Unmarshal(value, &media) != nil || media != string(OCIIndex) {
			return nil, nil, nil, types.NewError(types.InvalidArgument)
		}
	}
	var rows []json.RawMessage
	if json.Unmarshal(fields["manifests"], &rows) != nil || rows == nil {
		return nil, nil, nil, types.NewError(types.InvalidArgument)
	}
	return fields, rows, &digest, nil
}

func (c *Referrers) tagDescriptor(subject, content types.ResourceRef, input ManifestWriteOptions) (json.RawMessage, error) {
	if e := c.client.Manifests.valid(content); e != nil {
		return nil, e
	}
	if content.Kind != "manifest_digest" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if !reflect.DeepEqual(content.Parent, subject.Parent) {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if input.MediaType != OCIManifest && input.MediaType != OCIIndex {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	raw := input.Payload.Reveal()
	if uint64(len(raw)) > c.client.runtime.StructuredLimit() {
		return nil, types.NewError(types.ResponseLimit)
	}
	expected, _ := ParseDigest(content.ID)
	actual, e := hashBytes(raw, expected.Algorithm)
	if e != nil || actual != expected {
		return nil, types.NewError(types.InvalidArgument)
	}
	requested, e := manifestSubject(raw, input.MediaType, *content.Parent)
	if e != nil {
		return nil, e
	}
	if requested == nil || requested.String() != subject.ID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	fields, e := nonnullObject(raw, []string{"schemaVersion", "mediaType", "artifactType", "annotations", "subject"})
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	var version types.Integer
	if json.Unmarshal(fields["schemaVersion"], &version) != nil || version.String() != "2" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if value, ok := fields["mediaType"]; ok {
		var media string
		if json.Unmarshal(value, &media) != nil || media != string(input.MediaType) {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	var artifact string
	if value, ok := fields["artifactType"]; ok {
		if json.Unmarshal(value, &artifact) != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	if artifact == "" && input.MediaType == OCIManifest {
		var config struct {
			MediaType *string `json:"mediaType"`
		}
		if json.Unmarshal(fields["config"], &config) != nil || config.MediaType == nil || *config.MediaType == "" {
			return nil, types.NewError(types.InvalidArgument)
		}
		artifact = *config.MediaType
	}
	var artifactType *string
	if artifact != "" {
		if _, _, e := mime.ParseMediaType(artifact); e != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		artifactType = &artifact
	}
	annotations := fields["annotations"]
	if annotations != nil {
		var values map[string]*string
		if json.Unmarshal(annotations, &values) != nil || values == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		for _, v := range values {
			if v == nil {
				return nil, types.NewError(types.InvalidArgument)
			}
		}
	}
	size, _ := types.ParseInteger(strconv.Itoa(len(raw)))
	encoded, e := json.Marshal(tagDescriptorWire{MediaType: string(input.MediaType), Digest: content.ID, Size: size, ArtifactType: artifactType, Annotations: annotations})
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return encoded, nil
}
