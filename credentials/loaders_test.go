package credentials_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func request() types.AuthRequest {
	return types.AuthRequest{Scope: types.Scope{AccountID: "test-account", ProjectID: "project-one"}, Mode: types.IAMProject, Service: "compute", Audience: credentials.SelectelAudience}
}

func TestExplicitEnvironmentRevisionAndIsolation(t *testing.T) {
	t.Setenv("SDK_TEST_ACCOUNT_ID", "test-account")
	t.Setenv("SDK_TEST_USERNAME", "test-user")
	t.Setenv("SDK_TEST_PASSWORD", "synthetic-one")
	s, err := credentials.FromEnvironment("SDK_TEST_")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Lookup(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Lookup(context.Background(), request())
	if err != nil || m.Revision != again.Revision {
		t.Fatal("unstable revision")
	}
	t.Setenv("SDK_TEST_PASSWORD", "synthetic-two")
	rotated, err := s.Lookup(context.Background(), request())
	if err != nil || rotated.Revision == m.Revision || rotated.Identity != m.Identity {
		t.Fatal("rotation not identified")
	}
	r := request()
	r.Scope.AccountID = "different-account"
	if _, err := s.Lookup(context.Background(), r); types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatal("cross-account material allowed")
	}
}

func TestNamedProfileAndMalformedInputAreSafe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "profiles.json")
	data := `{"selected":{"account_id":"test-account","username":"test-user","password":"synthetic-one"},"other":{"account_id":"other-account","username":"other-user","password":"synthetic-two"}}`
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := credentials.FromProfile(p, "selected")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Lookup(context.Background(), request())
	if err != nil || m.Binding.AccountID != "test-account" {
		t.Fatal("wrong profile")
	}
	if err := os.WriteFile(p, []byte(`{"selected":{"password":"synthetic-private-marker"`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(context.Background(), request()); types.ErrorCode(err) != types.CredentialsUnavailable {
		t.Fatal("malformed source accepted")
	}
}

func TestCredentialSourceCancellationAndAmbiguousIAM(t *testing.T) {
	t.Setenv("SDK_TEST_ACCOUNT_ID", "test-account")
	t.Setenv("SDK_TEST_USERNAME", "test-user")
	t.Setenv("SDK_TEST_PASSWORD", "synthetic-password")
	t.Setenv("SDK_TEST_IAM_TOKEN", "synthetic-token")
	t.Setenv("SDK_TEST_TOKEN_EXPIRES_AT", "2099-01-01T00:00:00Z")
	s, err := credentials.FromEnvironment("SDK_TEST_")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(context.Background(), request()); types.ErrorCode(err) != types.CredentialsUnavailable {
		t.Fatal("ambiguous IAM accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Lookup(ctx, request()); types.ErrorCode(err) != types.Canceled {
		t.Fatal("cancellation ignored")
	}
}
