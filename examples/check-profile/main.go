// Check explicitly selected IAM credentials, without reading secret values or provisioning.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func main() {
	path := flag.String("profiles", "", "Explicit local profiles JSON path")
	name := flag.String("profile", "", "Profile name")
	account := flag.String("account", "", "Account number")
	project := flag.String("project", "", "Optional exact project binding")
	flag.Parse()
	if err := check(*path, *name, *account, *project); err != nil {
		fmt.Fprintf(os.Stderr, "SDK error: %s", types.ErrorCode(err))
		var detail *types.SDKError
		if errors.As(err, &detail) {
			fmt.Fprintf(os.Stderr, " http_status=%d", detail.HTTPStatus)
		}
		fmt.Fprintln(os.Stderr, "; installation and saved profiles are retained")
		os.Exit(1)
	}
}

func check(path, name, account, project string) error {
	source, err := credentials.FromProfile(path, name)
	if err != nil {
		return err
	}
	client, err := sdk.New(sdk.Config{AccountID: account, Credentials: source})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mode := types.IAMAccount
	scope := types.Scope{AccountID: account, ProjectID: project}
	if project != "" {
		mode = types.IAMProject
	}
	result, err := client.Authenticate(ctx, types.AuthRequest{
		Scope: scope, Mode: mode, Service: "compute", Audience: credentials.SelectelAudience,
	})
	if err != nil {
		return err
	}
	fmt.Printf("auth=ok verification=%s\n", result.Verification)
	fmt.Println("IAM authentication does not verify permissions for every service operation.")
	return nil
}
