// Package credentials provides explicit, context-aware credential sources.
package credentials

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/festy23/selectel-sdk/types"
)

const SelectelAudience = "selectel"

type Scheme string

const (
	IAMPassword Scheme = "iam_password"
	IAMToken    Scheme = "iam_token"
	StaticToken Scheme = "static_token"
	S3          Scheme = "s3_sigv4"
	Basic       Scheme = "basic"
)

// Source implementations must change Revision whenever their material changes.
// Identity and Revision are opaque, non-sensitive identifiers.
type Source interface {
	Lookup(context.Context, types.AuthRequest) (Material, error)
}
type SourceFunc func(context.Context, types.AuthRequest) (Material, error)

func (f SourceFunc) Lookup(ctx context.Context, request types.AuthRequest) (Material, error) {
	return f(ctx, request)
}

type Material struct {
	Identity     string
	Revision     string
	Binding      types.Scope
	Audience     string
	Scheme       Scheme
	Username     types.ProtectedValue
	Password     types.ProtectedValue
	Token        types.ProtectedValue
	ExpiresAt    time.Time
	AccessKey    types.ProtectedValue
	SecretKey    types.ProtectedValue
	SessionToken types.ProtectedValue
}

func (Material) String() string               { return "[REDACTED]" }
func (Material) GoString() string             { return "[REDACTED]" }
func (Material) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[REDACTED]") }
func (Material) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

func (m Material) Validate(request types.AuthRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if m.Identity == "" || m.Revision == "" || m.Binding.AccountID == "" {
		return types.NewError(types.CredentialsUnavailable)
	}
	if m.Binding.AccountID != request.Scope.AccountID || m.Audience != request.Audience ||
		(m.Binding.ProjectID != "" && m.Binding.ProjectID != request.Scope.ProjectID) ||
		(m.Binding.Region != "" && m.Binding.Region != request.Scope.Region) ||
		(m.Binding.Pool != "" && m.Binding.Pool != request.Scope.Pool) {
		return types.NewError(types.ScopeMismatch)
	}
	switch request.Mode {
	case types.IAMAccount, types.IAMProject:
		if m.Scheme == IAMPassword && !m.Username.Empty() && !m.Password.Empty() {
			return nil
		}
		if m.Scheme == IAMToken && !m.Token.Empty() && !m.ExpiresAt.IsZero() {
			if m.Binding.ProjectID != request.Scope.ProjectID {
				return types.NewError(types.ScopeMismatch)
			}
			return nil
		}
	case types.StaticToken:
		if m.Scheme == StaticToken && !m.Token.Empty() {
			return nil
		}
	case types.S3SigV4:
		if m.Scheme == S3 && !m.AccessKey.Empty() && !m.SecretKey.Empty() && m.Binding.ProjectID == request.Scope.ProjectID {
			return nil
		}
	case types.Basic:
		if m.Scheme == Basic && !m.Username.Empty() && !m.Password.Empty() && m.Binding.ProjectID == request.Scope.ProjectID {
			return nil
		}
	case types.ProtocolSpecific:
		return types.NewError(types.CapabilityUnavailable)
	}
	return types.NewError(types.CredentialsUnavailable)
}

func ID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", types.NewError(types.CredentialsUnavailable)
	}
	return hex.EncodeToString(b[:]), nil
}
