// Read one account-scoped Global Router page; this example performs no mutation.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/globalrouter"
	"github.com/festy23/selectel-sdk/types"
)

func main() {
	path := flag.String("profiles", "", "Explicit profiles JSON path")
	name := flag.String("profile", "", "Profile name")
	account := flag.String("account", "", "Account number")
	flag.Parse()
	if err := read(*path, *name, *account); err != nil {
		fmt.Fprintln(os.Stderr, "SDK error:", types.ErrorCode(err))
		os.Exit(1)
	}
}
func read(path, name, account string) error {
	source, err := credentials.FromProfile(path, name)
	if err != nil {
		return err
	}
	c, err := sdk.New(sdk.Config{AccountID: account, Credentials: source})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	limit, _ := types.ParseInteger("10")
	result, err := c.Account().GlobalRouter().Routers.ListPage(ctx, globalrouter.PageOptions{Limit: &limit}, types.CallOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("records=%d (one page; collection completeness unknown)\n", len(result.Data.Items))
	return nil
}
