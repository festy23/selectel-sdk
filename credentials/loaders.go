package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/festy23/selectel-sdk/types"
)

// Store is the caller's keyring adapter. No OS keyring is implicitly selected.
type Store interface {
	Get(context.Context, string) ([]byte, error)
}

type record struct {
	AccountID      string `json:"account_id"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	ProjectID      string `json:"project_id"`
	IAMToken       string `json:"iam_token"`
	TokenExpiresAt string `json:"token_expires_at"`
	StaticToken    string `json:"static_token"`
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key"`
	BasicUser      string `json:"basic_user"`
	BasicPassword  string `json:"basic_password"`
}

type loader struct {
	identity string
	load     func(context.Context) ([]byte, error)
	mu       sync.Mutex
	digest   [32]byte
	revision string
}

func (*loader) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[REDACTED]") }
func (*loader) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

func newLoader(load func(context.Context) ([]byte, error)) (Source, error) {
	id, err := ID()
	if err != nil {
		return nil, err
	}
	return &loader{identity: id, load: load}, nil
}

func (l *loader) Lookup(ctx context.Context, req types.AuthRequest) (Material, error) {
	if ctx == nil {
		return Material{}, types.NewError(types.InvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return Material{}, types.ContextError(err)
	}
	raw, err := l.load(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return Material{}, types.ContextError(ctx.Err())
		}
		return Material{}, types.NewError(types.CredentialsUnavailable)
	}
	if ctx.Err() != nil {
		return Material{}, types.ContextError(ctx.Err())
	}
	var r record
	if json.Unmarshal(raw, &r) != nil || r.AccountID == "" {
		return Material{}, types.NewError(types.CredentialsUnavailable)
	}
	digest := sha256.Sum256(raw)
	l.mu.Lock()
	if l.revision == "" || digest != l.digest {
		id, err := ID()
		if err != nil {
			l.mu.Unlock()
			return Material{}, err
		}
		l.digest, l.revision = digest, id
	}
	revision := l.revision
	l.mu.Unlock()
	m := Material{Identity: l.identity, Revision: revision, Binding: types.Scope{AccountID: r.AccountID, ProjectID: r.ProjectID}, Audience: SelectelAudience}
	p := func(s string) types.ProtectedValue { return types.Protect([]byte(s)) }
	switch req.Mode {
	case types.IAMAccount, types.IAMProject:
		if r.IAMToken != "" || r.TokenExpiresAt != "" {
			if r.Username != "" || r.Password != "" {
				return Material{}, types.NewError(types.CredentialsUnavailable)
			}
			t, err := time.Parse(time.RFC3339Nano, r.TokenExpiresAt)
			if err != nil || r.IAMToken == "" {
				return Material{}, types.NewError(types.CredentialsUnavailable)
			}
			m.Scheme, m.Token, m.ExpiresAt = IAMToken, p(r.IAMToken), t
		} else {
			m.Scheme, m.Username, m.Password = IAMPassword, p(r.Username), p(r.Password)
		}
	case types.StaticToken:
		m.Scheme, m.Token = StaticToken, p(r.StaticToken)
	case types.S3SigV4:
		m.Scheme, m.AccessKey, m.SecretKey = S3, p(r.S3AccessKey), p(r.S3SecretKey)
	case types.Basic:
		m.Scheme, m.Username, m.Password = Basic, p(r.BasicUser), p(r.BasicPassword)
	default:
		return Material{}, types.NewError(types.CapabilityUnavailable)
	}
	if err := m.Validate(req); err != nil {
		return Material{}, err
	}
	return m, nil
}

func FromEnvironment(prefix string) (Source, error) {
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(prefix) {
		return nil, types.NewError(types.InvalidArgument)
	}
	return newLoader(func(ctx context.Context) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r := record{AccountID: os.Getenv(prefix + "ACCOUNT_ID"), Username: os.Getenv(prefix + "USERNAME"), Password: os.Getenv(prefix + "PASSWORD"), ProjectID: os.Getenv(prefix + "PROJECT_ID"), IAMToken: os.Getenv(prefix + "IAM_TOKEN"), TokenExpiresAt: os.Getenv(prefix + "TOKEN_EXPIRES_AT"), StaticToken: os.Getenv(prefix + "STATIC_TOKEN"), S3AccessKey: os.Getenv(prefix + "S3_ACCESS_KEY"), S3SecretKey: os.Getenv(prefix + "S3_SECRET_KEY"), BasicUser: os.Getenv(prefix + "BASIC_USER"), BasicPassword: os.Getenv(prefix + "BASIC_PASSWORD")}
		return json.Marshal(r)
	})
}

// FromProfile explicitly binds a path/name. Reads occur at Lookup with its context.
// The JSON root maps profile names directly to records.
func FromProfile(path, name string) (Source, error) {
	if path == "" || name == "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	return newLoader(func(ctx context.Context) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 1024*1024 || ctx.Err() != nil {
			return nil, types.NewError(types.CredentialsUnavailable)
		}
		var records map[string]json.RawMessage
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, err
		}
		raw, ok := records[name]
		if !ok {
			return nil, types.NewError(types.CredentialsUnavailable)
		}
		return raw, nil
	})
}

func FromKeyring(store Store, entry string) (Source, error) {
	if store == nil || entry == "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	return newLoader(func(ctx context.Context) ([]byte, error) {
		data, err := store.Get(ctx, entry)
		if len(data) > 1024*1024 {
			return nil, types.NewError(types.CredentialsUnavailable)
		}
		return data, err
	})
}
