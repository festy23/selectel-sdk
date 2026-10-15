package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"io"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

// Operation is an internal, compiled binding. It is never a public raw executor.
type Operation struct {
	ID, Service, APIGeneration, Method     string
	Effect                                 string
	Auth                                   types.AuthMode
	Alternatives                           []types.AuthMode
	EndpointID, FixedEndpoint, ServiceType string
	RequireProject, RequireLocation        bool
	Success, Rejected                      []int
	NoContentSuccess                       []int
	ErrorCodes                             map[int]types.Code
	Headers                                http.Header
	SigV4Service, ErrorProtocol            string
}
type Request struct {
	Scope              types.Scope
	Path               string
	Query              url.Values
	Body               []byte
	BodyStream         io.Reader
	BodySize           int64
	Transfer           bool
	Options            types.CallOptions
	Refs               []types.ResourceRef
	PinnedEndpoint     *types.EndpointBinding
	ExpectedEndpointID string
	Validate           func() error
}
type Decoded[T any] struct {
	Data         T
	Refs         []types.ResourceRef
	OperationRef *types.ResourceRef
	PartialRead  bool
}
type Decoder[T any] func([]byte, types.EndpointBinding) (Decoded[T], error)

// ResponseDecoder is a private service binding seam. It consumes the response
// synchronously with the common budget context; response bodies never escape runtime.
type ResponseDecoder[T any] func(context.Context, io.Reader, types.EndpointBinding, int, http.Header) (Decoded[T], error)

// DoJSON applies one budget to validation, credentials, auth, requests and retries.
// A positive acknowledgement is preserved even if decoding the body fails.
func DoJSON[T any](r *Runtime, ctx context.Context, op Operation, in Request, decode Decoder[T]) (types.Result[T], error) {
	var adapter ResponseDecoder[T]
	if decode != nil {
		adapter = func(_ context.Context, body io.Reader, endpoint types.EndpointBinding, status int, _ http.Header) (Decoded[T], error) {
			raw, e := readResponse(body, r.config.MaxJSONBytes)
			if e != nil {
				return Decoded[T]{}, e
			}
			if len(op.NoContentSuccess) > 0 && (len(bytes.TrimSpace(raw)) == 0) != containsStatus(op.NoContentSuccess, status) {
				return Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			return decode(raw, endpoint)
		}
	}
	return DoResponse(r, ctx, op, in, adapter)
}

// DoResponse preserves the shared validation/auth/receipt execution algorithm for
// typed header and streaming bindings. No public raw executor is exposed.
func DoResponse[T any](r *Runtime, ctx context.Context, op Operation, in Request, decode ResponseDecoder[T]) (out types.Result[T], err error) {
	started := time.Now().UTC()
	out.Metadata = types.Metadata{Scope: in.Scope, APIGeneration: op.APIGeneration, RequestIDs: []string{}}
	var attempt uint
	var status int
	if op.Effect == "mutation" {
		id, idErr := uuid()
		if idErr != nil {
			return out, idErr
		}
		out.Receipt = &types.Receipt{SchemaVersion: 1, CallID: id, OperationID: op.ID, Scope: in.Scope, Outcome: types.NotSent, Recovery: "unavailable", ResourceRefs: []types.ResourceRef{}}
		// Local validation failures still return a revision-bound terminal receipt.
		if r != nil {
			if capability, e := r.catalog.Describe(op.ID); e == nil {
				out.Receipt.ContractRevision = capability.ContractRevision
			}
		}
	}
	defer func() {
		out.Metadata.ObservedAt = time.Now().UTC()
		if out.Receipt != nil {
			out.Receipt.CompletedAt = out.Metadata.ObservedAt
			out.Receipt.Attempts = attempt
			out.Receipt.RequestIDs = append([]string(nil), out.Metadata.RequestIDs...)
			if out.Receipt.Outcome == types.Unknown {
				cause := types.ErrorCode(err)
				e := types.NewError(types.UnknownOutcome)
				e.CauseCode = cause
				err = e
			}
		}
		if err != nil {
			var e *types.SDKError
			if !errors.As(err, &e) {
				e = types.ContextError(err)
			} else {
				copy := *e
				e = &copy
			}
			e.OperationID = op.ID
			e.Scope = in.Scope
			e.EndpointID = out.Metadata.EndpointID
			e.Receipt = out.Receipt
			e.RequestIDs = append([]string(nil), out.Metadata.RequestIDs...)
			if e.HTTPStatus == 0 {
				e.HTTPStatus = status
			}
			err = e
		}
		if r != nil {
			event := Event{OperationID: op.ID, Scope: in.Scope, APIGeneration: op.APIGeneration, EndpointID: out.Metadata.EndpointID, Attempt: attempt, Latency: time.Since(started), HTTPStatus: status, Code: types.ErrorCode(err), RequestIDs: append([]string(nil), out.Metadata.RequestIDs...), Complete: err == nil}
			if out.Receipt != nil {
				event.Outcome = out.Receipt.Outcome
			}
			r.observe(event)
		}
	}()
	if r == nil || ctx == nil || decode == nil {
		return out, types.NewError(types.InvalidArgument)
	}
	budget := r.config.CallTimeout
	if in.Transfer {
		budget = r.config.TransferTimeout
	}
	call, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if call.Err() != nil {
		return out, types.ContextError(call.Err())
	}
	if err := in.Scope.Validate(); err != nil {
		return out, err
	}
	if in.Scope.AccountID != r.config.AccountID {
		return out, types.NewError(types.ScopeMismatch)
	}
	if op.ID == "" || op.Service == "" || op.APIGeneration == "" || op.Method == "" || len(op.Success) == 0 || (op.Effect != "read" && op.Effect != "mutation") {
		return out, types.NewError(types.CapabilityUnavailable)
	}
	if (op.RequireProject && in.Scope.ProjectID == "") || (op.RequireLocation && in.Scope.Region == "" && in.Scope.Pool == "") {
		return out, types.NewError(types.InvalidArgument)
	}
	if in.Options.IdempotencyKey != "" {
		return out, types.NewError(types.InvalidArgument)
	} // No undocumented replay/header injection.
	if in.BodyStream != nil && (len(in.Body) > 0 || in.BodySize < 0 || op.Effect != "mutation" || op.Auth == types.S3SigV4) {
		return out, types.NewError(types.InvalidArgument)
	}
	if in.Validate != nil {
		if err := in.Validate(); err != nil {
			return out, err
		}
	}
	if !strings.HasPrefix(in.Path, "/") || strings.ContainsAny(in.Path, "?#") {
		return out, types.NewError(types.InvalidArgument)
	}
	for _, ref := range in.Refs {
		if err := ref.Validate(); err != nil {
			return out, err
		}
		if ref.Scope != in.Scope || ref.Service != op.Service || ref.APIGeneration != op.APIGeneration {
			return out, types.NewError(types.ScopeMismatch)
		}
	}
	capability, capErr := r.catalog.Describe(op.ID)
	if capErr != nil {
		return out, capErr
	}
	if out.Receipt != nil {
		out.Receipt.ContractRevision = capability.ContractRevision
		out.Receipt.ResourceRefs = cloneRefs(in.Refs)
		if len(in.Refs) > 0 {
			out.Receipt.Recovery = "resource"
		}
	}
	mode := op.Auth
	if in.Options.AuthMode != "" && in.Options.AuthMode != mode {
		allowed := false
		for _, a := range op.Alternatives {
			if a == in.Options.AuthMode {
				allowed = true
			}
		}
		if !allowed {
			return out, types.NewError(types.InvalidArgument)
		}
		mode = in.Options.AuthMode
	}
	authReq := types.AuthRequest{Scope: in.Scope, Mode: mode, Service: op.Service, Audience: credentials.SelectelAudience}
	entry, authErr := r.authorize(call, authReq)
	if authErr != nil {
		return out, authErr
	}
	if out.Receipt != nil {
		out.Receipt.CredentialIdentity = entry.material.Identity
	}
	endpoint, endpointErr := r.resolve(op, in, entry)
	if endpointErr != nil {
		return out, endpointErr
	}
	out.Metadata.EndpointID = endpoint.ID
	if out.Receipt != nil {
		copy := endpoint
		out.Receipt.Endpoint = &copy
	}
	maxAttempts := uint(1)
	if op.Effect == "read" {
		maxAttempts = r.config.ReadMaxAttempts
	}
	refreshed := false
	for attempt < maxAttempts {
		if call.Err() != nil {
			return out, types.ContextError(call.Err())
		}
		u, parseErr := url.Parse(strings.TrimRight(endpoint.URL, "/") + in.Path)
		if parseErr != nil {
			return out, types.NewError(types.InvalidArgument)
		}
		u.RawQuery = in.Query.Encode()
		var body io.Reader = bytes.NewReader(in.Body)
		if in.BodyStream != nil {
			body = io.NopCloser(in.BodyStream)
		}
		req, requestErr := http.NewRequestWithContext(call, op.Method, u.String(), body)
		if requestErr != nil {
			return out, types.NewError(types.InvalidArgument)
		}
		if in.BodyStream != nil {
			req.ContentLength = in.BodySize
		}
		for k, values := range op.Headers {
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "X-Auth-Token") || strings.EqualFold(k, "X-Token") || strings.EqualFold(k, "Host") {
				return out, types.NewError(types.InvalidArgument)
			}
			for _, value := range values {
				req.Header.Add(k, value)
			}
		}
		if len(in.Body) > 0 && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if headerErr := authorizeHeader(req, entry, op, in); headerErr != nil {
			return out, headerErr
		}
		attempt++
		if out.Receipt != nil {
			when := time.Now().UTC()
			if out.Receipt.SubmittedAt == nil {
				out.Receipt.SubmittedAt = &when
			}
			out.Receipt.Outcome = types.Unknown
		}
		resp, transportErr := r.http.Do(req)
		if transportErr != nil {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if call.Err() != nil {
				return out, types.ContextError(call.Err())
			}
			if op.Effect == "read" && attempt < maxAttempts && temporary(transportErr) {
				if retryErr := r.retryDelay(call, attempt, ""); retryErr == nil {
					continue
				} else {
					return out, retryErr
				}
			}
			return out, types.NewError(types.TransportError)
		}
		status = resp.StatusCode
		out.Metadata.RequestIDs = append(out.Metadata.RequestIDs, requestIDs(resp.Header)...)
		if containsStatus(op.Success, status) {
			if out.Receipt != nil {
				out.Receipt.Outcome = types.Accepted
			}
			closeBody := sync.OnceFunc(func() { _ = resp.Body.Close() })
			stopClose := context.AfterFunc(call, closeBody)
			decoded, decodeErr := decode(call, resp.Body, endpoint, status, resp.Header.Clone())
			stopClose()
			closeBody()
			if decodeErr != nil && call.Err() != nil {
				decodeErr = types.ContextError(call.Err())
			}
			if out.Receipt != nil {
				for _, ref := range decoded.Refs {
					if ref.Validate() != nil || ref.Scope != in.Scope || ref.Service != op.Service || ref.APIGeneration != op.APIGeneration || ref.EndpointID != endpoint.ID || ref.EndpointURL != endpoint.URL {
						return out, types.NewError(types.ScopeMismatch)
					}
				}
				for _, ref := range cloneRefs(decoded.Refs) {
					present := false
					for _, known := range out.Receipt.ResourceRefs {
						if reflect.DeepEqual(known, ref) {
							present = true
							break
						}
					}
					if !present {
						out.Receipt.ResourceRefs = append(out.Receipt.ResourceRefs, ref)
					}
				}
				if decoded.OperationRef != nil {
					return out, types.NewError(types.CapabilityUnavailable)
				} // Task validation is bound per service before adding it.
				if len(out.Receipt.ResourceRefs) > 0 {
					out.Receipt.Recovery = "resource"
				}
				out.Data = decoded.Data
			} else if decodeErr == nil || (decoded.PartialRead && (types.ErrorCode(decodeErr) == types.ResponseLimit || in.Transfer)) {
				out.Data = decoded.Data
			}
			if decodeErr != nil {
				var sdkError *types.SDKError
				if errors.As(decodeErr, &sdkError) {
					return out, sdkError
				}
				return out, types.NewError(types.ProtocolError)
			}
			return out, nil
		}
		providerError := statusError(status)
		if code, ok := op.ErrorCodes[status]; ok {
			providerError = types.NewError(code)
			providerError.HTTPStatus = status
		}
		if op.ErrorProtocol == "cloudwatch" {
			raw, readErr := readResponse(resp.Body, r.config.MaxJSONBytes)
			if readErr != nil {
				resp.Body.Close()
				if call.Err() != nil {
					return out, types.ContextError(call.Err())
				}
				return out, readErr
			}
			providerError = cloudwatchError(status, raw)
		}
		resp.Body.Close() // Provider error payloads are not retained or logged.
		if out.Receipt != nil && containsStatus(op.Rejected, status) {
			out.Receipt.Outcome = types.Rejected
		}
		if status == http.StatusUnauthorized {
			r.invalidate(entry, in.Scope)
			if op.Effect == "read" && !refreshed && entry.material.Scheme == credentials.IAMPassword && attempt < maxAttempts {
				refreshed = true
				entry, authErr = r.authorize(call, authReq)
				if authErr != nil {
					return out, authErr
				}
				continue
			}
		}
		if op.Effect == "read" && retryStatus(status) && attempt < maxAttempts {
			if retryErr := r.retryDelay(call, attempt, resp.Header.Get("Retry-After")); retryErr == nil {
				continue
			} else if types.ErrorCode(retryErr) == types.Canceled || types.ErrorCode(retryErr) == types.DeadlineExceeded {
				return out, retryErr
			}
			e := providerError
			e.Reason = "retry_budget_exhausted"
			return out, e
		}
		return out, providerError
	}
	return out, types.NewError(types.ProviderError)
}

func (r *Runtime) resolve(op Operation, in Request, entry *authEntry) (types.EndpointBinding, error) {
	key := types.EndpointKey{Service: op.Service, APIGeneration: op.APIGeneration, Scope: in.Scope}
	endpoint, override := r.config.EndpointOverrides[key]
	if !override {
		endpoint = types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: op.APIGeneration, Scope: in.Scope, Audience: credentials.SelectelAudience}
		if op.ServiceType != "" {
			region := in.Scope.Region
			if in.Scope.Pool != "" {
				region = in.Scope.Pool
			}
			matches := map[string]catalogEndpoint{}
			for _, service := range entry.catalog {
				if service.Type != op.ServiceType {
					continue
				}
				for _, ep := range service.Endpoints {
					epRegion := ep.RegionID
					if epRegion == "" {
						epRegion = ep.Region
					}
					if ep.Interface == "public" && epRegion == region {
						matches[ep.URL] = ep
					}
				}
			}
			if len(matches) != 1 {
				return types.EndpointBinding{}, types.NewError(types.EndpointUnavailable)
			}
			for _, ep := range matches {
				endpoint.URL, endpoint.ID = ep.URL, ep.ID
			}
		}
	}
	if endpoint.ID == "" || endpoint.Scope != in.Scope || endpoint.APIGeneration != op.APIGeneration || endpoint.Audience != credentials.SelectelAudience {
		return types.EndpointBinding{}, types.NewError(types.ScopeMismatch)
	}
	if err := types.ValidateEndpoint(endpoint.URL); err != nil {
		return types.EndpointBinding{}, err
	}
	if in.PinnedEndpoint != nil && *in.PinnedEndpoint != endpoint {
		return types.EndpointBinding{}, types.NewError(types.EndpointUnavailable)
	}
	if in.ExpectedEndpointID != "" && in.ExpectedEndpointID != endpoint.ID {
		return types.EndpointBinding{}, types.NewError(types.EndpointUnavailable)
	}
	return endpoint, nil
}

func authorizeHeader(req *http.Request, e *authEntry, op Operation, in Request) error {
	switch e.material.Scheme {
	case credentials.IAMPassword, credentials.IAMToken:
		req.Header.Set("X-Auth-Token", string(e.token.Reveal()))
	case credentials.StaticToken:
		req.Header.Set("X-Token", string(e.material.Token.Reveal()))
	case credentials.Basic:
		req.SetBasicAuth(string(e.material.Username.Reveal()), string(e.material.Password.Reveal()))
	case credentials.S3:
		if op.SigV4Service == "" || in.Scope.Pool == "" {
			return types.NewError(types.CapabilityUnavailable)
		}
		digest := sha256.Sum256(in.Body)
		material := aws.Credentials{AccessKeyID: string(e.material.AccessKey.Reveal()), SecretAccessKey: string(e.material.SecretKey.Reveal()), SessionToken: string(e.material.SessionToken.Reveal())}
		if err := v4.NewSigner().SignHTTP(req.Context(), material, req, hex.EncodeToString(digest[:]), op.SigV4Service, in.Scope.Pool, time.Now().UTC()); err != nil {
			return types.NewError(types.Unauthenticated)
		}
	default:
		return types.NewError(types.CapabilityUnavailable)
	}
	return nil
}
func cloudwatchError(status int, raw []byte) *types.SDKError {
	var wire struct {
		Type string `json:"__type"`
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &wire) != nil {
		return statusError(status)
	}
	code := wire.Type
	if code == "" {
		code = wire.Code
	}
	if i := strings.LastIndex(code, "#"); i >= 0 {
		code = code[i+1:]
	}
	known := map[string]types.Code{"ResourceNotFoundException": types.NotFound, "InvalidParameterException": types.InvalidArgument, "AccessDeniedException": types.PermissionDenied, "UnrecognizedClientException": types.Unauthenticated, "InvalidSignatureException": types.Unauthenticated, "ThrottlingException": types.RateLimited, "ServiceUnavailableException": types.ProviderError}
	if mapped, ok := known[code]; ok {
		e := types.NewError(mapped)
		e.HTTPStatus = status
		e.ProviderCode = code
		return e
	}
	return statusError(status)
}
func (r *Runtime) invalidate(entry *authEntry, scope types.Scope) {
	key := authKey{credentialKey: credentialKey{entry.material.Identity, scope.AccountID, scope.ProjectID}, revision: entry.material.Revision}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.cache[key]; ok && current.generation == entry.generation {
		delete(r.cache, key)
		r.cleanRevision(key.credentialKey)
	}
}
func (r *Runtime) observe(event Event) {
	if r.config.Observer == nil {
		return
	}
	defer func() { _ = recover() }()
	r.config.Observer(event)
}
func readResponse(body io.Reader, limit uint64) ([]byte, error) {
	if limit >= uint64(^uint64(0)>>1) {
		return nil, types.NewError(types.ResponseLimit)
	}
	raw, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return nil, types.NewError(types.TransportError)
	}
	if uint64(len(raw)) > limit {
		return nil, types.NewError(types.ResponseLimit)
	}
	return raw, nil
}
func containsStatus(statuses []int, status int) bool {
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}
func retryStatus(status int) bool {
	switch status {
	case 408, 429, 500, 502, 503, 504:
		return true
	}
	return false
}
func temporary(err error) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &host) {
		return false
	}
	var n net.Error
	if errors.As(err, &n) {
		return n.Timeout() || n.Temporary()
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
func (r *Runtime) retryDelay(ctx context.Context, attempt uint, retryAfter string) error {
	cap := r.config.RetryBaseDelay
	for i := uint(1); i < attempt && cap < r.config.RetryMaxDelay; i++ {
		if cap > r.config.RetryMaxDelay/2 {
			cap = r.config.RetryMaxDelay
			break
		}
		cap *= 2
	}
	if cap > r.config.RetryMaxDelay {
		cap = r.config.RetryMaxDelay
	}
	delay := time.Duration(mathrand.Int64N(int64(cap)))
	if seconds, err := strconv.ParseUint(retryAfter, 10, 32); err == nil {
		server := time.Duration(seconds) * time.Second
		if server > delay {
			delay = server
		}
	} else if date, err := http.ParseTime(retryAfter); err == nil {
		server := time.Until(date)
		if server > delay {
			delay = server
		}
	}
	if deadline, ok := ctx.Deadline(); ok && delay >= time.Until(deadline) {
		e := types.NewError(types.ProviderError)
		e.Reason = "retry_budget_exhausted"
		return e
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return types.ContextError(ctx.Err())
	case <-timer.C:
		return nil
	}
}
func requestIDs(header http.Header) []string {
	out := []string{}
	for _, key := range []string{"X-Request-Id", "X-Openstack-Request-Id", "X-Amzn-Requestid", "X-Trans-Id"} {
		for _, id := range header.Values(key) {
			if len(id) > 0 && len(id) <= 128 && !strings.ContainsAny(id, "\r\n\t ") {
				out = append(out, id)
			}
		}
	}
	return out
}
func uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", types.NewError(types.TransportError)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func cloneRefs(refs []types.ResourceRef) []types.ResourceRef {
	out := make([]types.ResourceRef, len(refs))
	for i, v := range refs {
		out[i] = v
		if v.Parent != nil {
			parents := cloneRefs([]types.ResourceRef{*v.Parent})
			out[i].Parent = &parents[0]
		}
	}
	return out
}
