package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

const identityURL = "https://cloud.api.selcloud.ru/identity/v3/auth/tokens"

type credentialKey struct{ identity, account, project string }
type authKey struct {
	credentialKey
	revision string
}
type authEntry struct {
	token      types.ProtectedValue
	material   credentials.Material
	metadata   types.AuthMetadata
	catalog    []catalogService
	expiry     time.Time
	generation string
	used       uint64
}
type authFlight struct {
	done  chan struct{}
	entry *authEntry
	err   error
}
type catalogService struct {
	ID, Type, Name string
	Endpoints      []catalogEndpoint `json:"endpoints"`
}
type catalogEndpoint struct {
	ID        string `json:"id"`
	Interface string `json:"interface"`
	Region    string `json:"region"`
	RegionID  string `json:"region_id"`
	URL       string `json:"url"`
}

func (r *Runtime) authorize(ctx context.Context, request types.AuthRequest) (*authEntry, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Scope.AccountID != r.config.AccountID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if !r.catalog.HasService(request.Service) {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if request.Audience != credentials.SelectelAudience {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if err := ctx.Err(); err != nil {
		return nil, types.ContextError(err)
	}
	if request.Mode == types.ProtocolSpecific {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	material, err := r.config.Credentials.Lookup(ctx, request)
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	if err != nil {
		if types.ErrorCode(err) == types.ScopeMismatch {
			return nil, types.NewError(types.ScopeMismatch)
		}
		return nil, types.NewError(types.CredentialsUnavailable)
	}
	if err := material.Validate(request); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if material.Scheme == credentials.S3 && !material.ExpiresAt.IsZero() && !now.Before(material.ExpiresAt) {
		return nil, types.NewError(types.Unauthenticated)
	}
	if material.Scheme != credentials.IAMPassword && material.Scheme != credentials.IAMToken {
		return &authEntry{material: material, metadata: types.AuthMetadata{Scope: request.Scope, Verification: "caller_asserted", Scheme: string(material.Scheme), AcquiredAt: now}}, nil
	}
	if material.Scheme == credentials.IAMToken && !now.Before(material.ExpiresAt) {
		return nil, types.NewError(types.Unauthenticated)
	}
	key := authKey{credentialKey: credentialKey{material.Identity, request.Scope.AccountID, request.Scope.ProjectID}, revision: material.Revision}
	r.mu.Lock()
	r.latest[key.credentialKey] = key.revision
	// Evict previous material generations for this exact credential scope.
	for k := range r.cache {
		if k.credentialKey == key.credentialKey && k.revision != key.revision {
			delete(r.cache, k)
		}
	}
	if entry, ok := r.cache[key]; ok && now.Before(entry.expiry.Add(-*r.config.RefreshSkew)) {
		r.clock++
		entry.used = r.clock
		r.mu.Unlock()
		return entry, nil
	}
	if flight, ok := r.flights[key]; ok {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, types.ContextError(ctx.Err())
		case <-flight.done:
			return flight.entry, flight.err
		}
	}
	flight := &authFlight{done: make(chan struct{})}
	r.flights[key] = flight
	r.mu.Unlock()
	entry, err := r.fetchIAM(ctx, request, material)
	r.mu.Lock()
	if err == nil && r.latest[key.credentialKey] != key.revision {
		entry = nil
		err = types.NewError(types.CredentialsUnavailable)
	}
	if err == nil {
		r.clock++
		entry.used = r.clock
		if uint(len(r.cache)) >= r.config.AuthCacheEntries {
			var oldest authKey
			var min uint64 = ^uint64(0)
			for k, v := range r.cache {
				if v.used < min {
					oldest, min = k, v.used
				}
			}
			delete(r.cache, oldest)
			r.cleanRevision(oldest.credentialKey)
		}
		r.cache[key] = entry
	}
	flight.entry, flight.err = entry, err
	delete(r.flights, key)
	r.cleanRevision(key.credentialKey)
	close(flight.done)
	r.mu.Unlock()
	return entry, err
}

// Called with mu held. Revision markers exist only for cache entries or live
// flights, rather than retaining every scope ever requested by a caller.
func (r *Runtime) cleanRevision(key credentialKey) {
	for k := range r.cache {
		if k.credentialKey == key {
			return
		}
	}
	for k := range r.flights {
		if k.credentialKey == key {
			return
		}
	}
	delete(r.latest, key)
}

func (r *Runtime) fetchIAM(ctx context.Context, request types.AuthRequest, material credentials.Material) (*authEntry, error) {
	method := http.MethodPost
	var body []byte
	if material.Scheme == credentials.IAMToken {
		method = http.MethodGet
	} else {
		identity := map[string]any{"methods": []string{"password"}, "password": map[string]any{"user": map[string]any{"name": string(material.Username.Reveal()), "domain": map[string]string{"name": request.Scope.AccountID}, "password": string(material.Password.Reveal())}}}
		scope := map[string]any{"domain": map[string]string{"name": request.Scope.AccountID}}
		if request.Scope.ProjectID != "" {
			scope = map[string]any{"project": map[string]string{"id": request.Scope.ProjectID}}
		}
		var err error
		body, err = json.Marshal(map[string]any{"auth": map[string]any{"identity": identity, "scope": scope}})
		if err != nil {
			return nil, types.NewError(types.ProtocolError)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, identityURL, bytes.NewReader(body))
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	req.Header.Set("Content-Type", "application/json")
	if material.Scheme == credentials.IAMToken {
		token := string(material.Token.Reveal())
		req.Header.Set("X-Auth-Token", token)
		req.Header.Set("X-Subject-Token", token)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, types.ContextError(ctx.Err())
		}
		return nil, types.NewError(types.TransportError)
	}
	defer resp.Body.Close()
	expected := http.StatusCreated
	if method == http.MethodGet {
		expected = http.StatusOK
	}
	if resp.StatusCode != expected {
		return nil, statusError(resp.StatusCode)
	}
	if r.config.MaxJSONBytes >= uint64(^uint64(0)>>1) {
		return nil, types.NewError(types.ResponseLimit)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(r.config.MaxJSONBytes)+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, types.ContextError(ctx.Err())
		}
		return nil, types.NewError(types.TransportError)
	}
	if uint64(len(raw)) > r.config.MaxJSONBytes {
		return nil, types.NewError(types.ResponseLimit)
	}
	var envelope struct {
		Token struct {
			ExpiresAt time.Time `json:"expires_at"`
			Domain    *struct {
				Name string `json:"name"`
			} `json:"domain"`
			Project *struct {
				ID     string `json:"id"`
				Domain struct {
					Name string `json:"name"`
				} `json:"domain"`
			} `json:"project"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Catalog []catalogService `json:"catalog"`
		} `json:"token"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	t := envelope.Token
	now := time.Now().UTC()
	if t.ExpiresAt.IsZero() || !now.Before(t.ExpiresAt) {
		return nil, types.NewError(types.ProtocolError)
	}
	if request.Scope.ProjectID != "" {
		if t.Project == nil {
			return nil, types.NewError(types.ProtocolError)
		}
		if t.Project.ID != request.Scope.ProjectID || t.Project.Domain.Name != request.Scope.AccountID {
			return nil, types.NewError(types.ScopeMismatch)
		}
	} else {
		if t.Domain == nil {
			return nil, types.NewError(types.ProtocolError)
		}
		if t.Domain.Name != request.Scope.AccountID || t.Project != nil {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	token := resp.Header.Get("X-Subject-Token")
	if method == http.MethodGet {
		token = string(material.Token.Reveal())
	}
	if token == "" {
		return nil, types.NewError(types.ProtocolError)
	}
	expires := t.ExpiresAt.UTC()
	if material.Scheme == credentials.IAMToken && material.ExpiresAt.Before(expires) {
		expires = material.ExpiresAt.UTC()
	}
	if !time.Now().Before(expires) {
		return nil, types.NewError(types.Unauthenticated)
	}
	gen, err := credentials.ID()
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	metadataMaterial := credentials.Material{Identity: material.Identity, Revision: material.Revision, Binding: material.Binding, Audience: material.Audience, Scheme: material.Scheme}
	return &authEntry{token: types.Protect([]byte(token)), material: metadataMaterial, metadata: types.AuthMetadata{Scope: types.Scope{AccountID: request.Scope.AccountID, ProjectID: request.Scope.ProjectID}, Verification: "provider_verified", Scheme: string(material.Scheme), SubjectID: t.User.ID, ExpiresAt: &expires, AcquiredAt: now}, catalog: t.Catalog, expiry: expires, generation: gen}, nil
}

func statusError(status int) *types.SDKError {
	code := types.ProviderError
	switch status {
	case 401:
		code = types.Unauthenticated
	case 403:
		code = types.PermissionDenied
	case 404:
		code = types.NotFound
	case 409:
		code = types.Conflict
	case 429:
		code = types.RateLimited
	}
	if status >= 300 && status < 400 {
		code = types.ProtocolError
	}
	e := types.NewError(code)
	e.HTTPStatus = status
	return e
}
