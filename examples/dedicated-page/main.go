// Read the account-scoped Dedicated location catalog; never provisions a server.
package main

import (
	"context"
	"flag"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
	"os"
	"time"
)

func main() {
	profiles := flag.String("profiles", "", "Explicit profiles JSON path")
	profile := flag.String("profile", "", "Profile name")
	account := flag.String("account", "", "Account number")
	flag.Parse()
	if e := read(*profiles, *profile, *account); e != nil {
		fmt.Fprintln(os.Stderr, "SDK error:", types.ErrorCode(e))
		os.Exit(1)
	}
}
func read(path, name, account string) error {
	source, e := credentials.FromProfile(path, name)
	if e != nil {
		return e
	}
	c, e := sdk.New(sdk.Config{AccountID: account, Credentials: source})
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	v, e := c.Account().Dedicated().ListLocations(ctx, types.CallOptions{})
	if e != nil {
		return e
	}
	if v.Data.Data != nil {
		fmt.Printf("records=%d (one bounded response; completeness unknown)\n", len(v.Data.Data.Result))
	}
	return nil
}
