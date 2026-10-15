package selectelsdk_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture varies a pinned source, then observes the rebuilt SDK only through
// its public Client. It neither replaces internal collaborators nor calls a cloud.
func TestCatalogSourceDriftInvalidatesPublicVerification(t *testing.T) {
	root := t.TempDir()
	files, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Split(string(files), "\x00") {
		if name == "" {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(dest, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "cmd", "driftprobe")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	program := `package main
import("context";"encoding/json";"net/http";"os";sdk "github.com/festy23/selectel-sdk";"github.com/festy23/selectel-sdk/credentials";"github.com/festy23/selectel-sdk/types")
type noHTTP struct{}
func(noHTTP)RoundTrip(*http.Request)(*http.Response,error){panic("unexpected HTTP")}
func main(){c,e:=sdk.New(sdk.Config{AccountID:"synthetic",Credentials:credentials.SourceFunc(func(context.Context,types.AuthRequest)(credentials.Material,error){panic("unexpected credentials")}),Transport:noHTTP{}});if e!=nil{panic(e)};if len(os.Args)>1 && os.Args[1]=="report"{if e=json.NewEncoder(os.Stdout).Encode(c.Catalog().Audit());e!=nil{panic(e)};return};if len(os.Args)>1 {for _,check:=range c.Catalog().Audit().BindingChecks{if check.OperationID=="billing.v3.balances.get"{if e=json.NewEncoder(os.Stdout).Encode(check);e!=nil{panic(e)};return}};panic("check missing")};r,e:=c.Catalog().Describe("billing.v3.balances.get");if e!=nil{panic(e)};if e=json.NewEncoder(os.Stdout).Encode(r);e!=nil{panic(e)}}`
	if err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-p=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, stderr.String())
		}
		return out
	}
	read := func() struct {
		Source       struct{ Revision string }
		Verification struct{ Level string }
	} {
		t.Helper()
		var row struct {
			Source       struct{ Revision string }
			Verification struct{ Level string }
		}
		if err := json.Unmarshal(run("run", "./cmd/driftprobe"), &row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := read()
	if before.Verification.Level != "offline" {
		t.Fatalf("fixture lacks prior proof: %+v", before)
	}
	file := filepath.Join(root, "catalog", "sources", "balance.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// A content revision changes even when this harmless whitespace preserves meaning.
	if err = os.WriteFile(file, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	run("run", "./internal/cataloggen")
	after := read()
	if after.Source.Revision == before.Source.Revision || after.Verification.Level != "none" {
		t.Fatalf("stale source verification retained: before=%+v after=%+v", before, after)
	}
	// Rebuild an intentionally stale mapping fixture; the observable audit must
	// distinguish declaration absence from a dangling covered-schema pointer.
	bindingFile := filepath.Join(root, "catalog", "bindings.json")
	bindingRaw, err := os.ReadFile(bindingFile)
	if err != nil {
		t.Fatal(err)
	}
	var bindings map[string]map[string]any
	if err = json.Unmarshal(bindingRaw, &bindings); err != nil {
		t.Fatal(err)
	}
	bindings["billing.v3.balances.get"]["go_method"] = "billing.Client.NoSuchMethod"
	bindings["billing.v3.balances.get"]["covered_schema_ref"] = "contracts/billing.v3.balances.get.json#/missing"
	bindingRaw, err = json.Marshal(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bindingFile, bindingRaw, 0600); err != nil {
		t.Fatal(err)
	}
	run("run", "./internal/cataloggen")
	var check struct{ Issues []string }
	if err = json.Unmarshal(run("run", "./cmd/driftprobe", "audit"), &check); err != nil {
		t.Fatal(err)
	}
	if strings.Join(check.Issues, ",") != "covered_schema_reference_missing,public_method_declaration_missing" {
		t.Fatalf("stale binding not diagnosed: %+v", check)
	}

	// A binding with no canonical source operation must remain visible as a
	// reconciliation problem, without fabricating a provider operation row.
	bindings["billing.v3.missing.get"] = bindings["billing.v3.balances.get"]
	bindingRaw, err = json.Marshal(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bindingFile, bindingRaw, 0600); err != nil {
		t.Fatal(err)
	}
	run("run", "./internal/cataloggen")
	var report struct {
		OrphanBindings []string `json:"orphan_bindings"`
		Sources        []struct{ Documented int }
	}
	if err = json.Unmarshal(run("run", "./cmd/driftprobe", "report"), &report); err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.OrphanBindings, ",") != "billing.v3.missing.get" {
		t.Fatalf("unmapped binding silently dropped: %+v", report.OrphanBindings)
	}
	count := 0
	for _, src := range report.Sources {
		count += src.Documented
	}
	if count != 191 {
		t.Fatalf("orphan became provider operation: %d", count)
	}

}
