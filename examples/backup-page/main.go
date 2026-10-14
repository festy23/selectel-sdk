// Read one explicitly scoped backup page. This example performs no mutations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/backups"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

func main() {
	profiles := flag.String("profiles", "", "Explicit profile JSON path")
	profile := flag.String("profile", "", "Profile name")
	account := flag.String("account", "", "Account")
	project := flag.String("project", "", "Project")
	region := flag.String("region", "", "Region")
	pool := flag.String("pool", "", "Pool")
	flag.Parse()
	if err := readPage(*profiles, *profile, *account, *project, *region, *pool); err != nil {
		fmt.Fprintln(os.Stderr, "SDK error:", types.ErrorCode(err))
		os.Exit(1)
	}
}
func readPage(path, name, account, project, region, pool string) error {
	source, err := credentials.FromProfile(path, name)
	if err != nil {
		return err
	}
	client, err := sdk.New(sdk.Config{AccountID: account, Credentials: source})
	if err != nil {
		return err
	}
	p, err := client.Project(project)
	if err != nil {
		return err
	}
	scoped, err := p.Location(region, pool)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	limit, _ := types.ParseInteger("10")
	page, err := scoped.Backups().Plans.ListPage(ctx, backups.PlansQuery{Limit: &limit}, types.CallOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("records=%d total=%s\n", len(page.Data.Plans), page.Data.Total.String())
	fmt.Println("One page only; no collection completeness or live coverage claim.")
	return nil
}
