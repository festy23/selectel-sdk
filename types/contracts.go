package types

import (
	"net/url"
	"time"
)

type Scope struct {
	AccountID string `json:"account_id"`
	ProjectID string `json:"project_id,omitempty"`
	Region    string `json:"region,omitempty"`
	Pool      string `json:"pool,omitempty"`
}

func (s Scope) Validate() error {
	if s.AccountID == "" || ((s.Region != "" || s.Pool != "") && s.ProjectID == "") {
		return NewError(InvalidArgument)
	}
	return nil
}

type AuthMode string

const (
	IAMAccount       AuthMode = "iam_account"
	IAMProject       AuthMode = "iam_project"
	StaticToken      AuthMode = "static_token"
	S3SigV4          AuthMode = "s3_sigv4"
	Basic            AuthMode = "basic"
	ProtocolSpecific AuthMode = "protocol_specific"
)

func (m AuthMode) Valid() bool {
	switch m {
	case IAMAccount, IAMProject, StaticToken, S3SigV4, Basic, ProtocolSpecific:
		return true
	}
	return false
}

type AuthRequest struct {
	Scope    Scope
	Mode     AuthMode
	Service  string
	Audience string
}

func (r AuthRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if !r.Mode.Valid() || r.Service == "" || r.Audience == "" {
		return NewError(InvalidArgument)
	}
	if (r.Mode == IAMAccount && r.Scope.ProjectID != "") || (r.Mode == IAMProject && r.Scope.ProjectID == "") {
		return NewError(ScopeMismatch)
	}
	return nil
}

type EndpointKey struct {
	Service       string
	APIGeneration string
	Scope         Scope
}
type EndpointBinding struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	APIGeneration string `json:"api_generation"`
	Scope         Scope  `json:"scope"`
	Audience      string `json:"audience"`
}

func ValidateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Opaque != "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return NewError(ProtocolError)
	}
	return nil
}

type ResourceRef struct {
	Scope         Scope        `json:"scope"`
	Service       string       `json:"service"`
	APIGeneration string       `json:"api_generation"`
	Kind          string       `json:"kind"`
	ID            string       `json:"id"`
	Parent        *ResourceRef `json:"parent,omitempty"`
	EndpointID    string       `json:"endpoint_id"`
	EndpointURL   string       `json:"endpoint_url"`
}

func (r ResourceRef) Validate() error { return r.validate(0) }
func (r ResourceRef) validate(depth int) error {
	if depth > 16 || r.Service == "" || r.APIGeneration == "" || r.Kind == "" || r.ID == "" || r.EndpointID == "" {
		return NewError(InvalidArgument)
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := ValidateEndpoint(r.EndpointURL); err != nil {
		return err
	}
	if r.Parent != nil {
		if r.Parent.Scope != r.Scope {
			return NewError(ScopeMismatch)
		}
		return r.Parent.validate(depth + 1)
	}
	return nil
}

type Outcome string

const (
	NotSent  Outcome = "not_sent"
	Rejected Outcome = "rejected"
	Accepted Outcome = "accepted"
	Unknown  Outcome = "unknown"
)

type Receipt struct {
	SchemaVersion        int              `json:"schema_version"`
	CallID               string           `json:"call_id"`
	OperationID          string           `json:"operation_id"`
	ContractRevision     string           `json:"contract_revision"`
	Scope                Scope            `json:"scope"`
	CredentialIdentity   string           `json:"credential_identity,omitempty"`
	Endpoint             *EndpointBinding `json:"endpoint,omitempty"`
	Outcome              Outcome          `json:"outcome"`
	SubmittedAt          *time.Time       `json:"submitted_at,omitempty"`
	CompletedAt          time.Time        `json:"completed_at"`
	Attempts             uint             `json:"attempts"`
	RequestIDs           []string         `json:"request_ids,omitempty"`
	ResourceRefs         []ResourceRef    `json:"resource_refs,omitempty"`
	ProviderOperationRef *ResourceRef     `json:"provider_operation_ref,omitempty"`
	Recovery             string           `json:"recovery"`
	Correlation          string           `json:"correlation,omitempty"`
}

type Metadata struct {
	Scope         Scope     `json:"scope"`
	EndpointID    string    `json:"endpoint_id"`
	APIGeneration string    `json:"api_generation"`
	ObservedAt    time.Time `json:"observed_at"`
	RequestIDs    []string  `json:"request_ids,omitempty"`
}

type Result[T any] struct {
	Data     T        `json:"data"`
	Metadata Metadata `json:"metadata"`
	Receipt  *Receipt `json:"receipt,omitempty"`
}

type Money struct {
	Raw      string  `json:"raw"`
	Currency *string `json:"currency,omitempty"`
	Unit     string  `json:"unit"`
	Scale    *int32  `json:"scale,omitempty"`
}

type CallOptions struct {
	AuthMode       AuthMode
	IdempotencyKey string
}

type AuthMetadata struct {
	Scope        Scope      `json:"scope"`
	Verification string     `json:"verification"`
	Scheme       string     `json:"scheme"`
	SubjectID    string     `json:"subject_id,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	AcquiredAt   time.Time  `json:"acquired_at"`
}
