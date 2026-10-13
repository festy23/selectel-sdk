# Go execution and explicit access

Resolve the repository root and work in the repository root; imports resolve to
the local module there. Put one-off Go programs in a temporary directory outside
Git, and run `go run /absolute/path/to/program.go` from the module directory.
The required Go version is in `go.mod`.

## Offline discovery

This program uses the public SDK catalog without an account, source or network
request. Change the service filter to the requested service.

```go
package main

import (
    "encoding/json"
    "os"

    "github.com/festy23/selectel-sdk/catalog"
)

func main() {
    c := catalog.Default()
    operations, err := c.List(catalog.Filter{Service: "mks"})
    if err != nil {
        panic(err)
    }
    if err := json.NewEncoder(os.Stdout).Encode(operations); err != nil {
        panic(err)
    }
}
```

`Describe(operationID)` returns one capability. `Artifact(ref)` returns the
referenced schema/contract bytes, including a JSON pointer fragment. Capability
status is separate from Verification.Level; inspect both. Catalog Services and
Summary give source inventory completeness without cloud access.

## Explicit credentials and bounded reads

Select the existing source and account provided by the user or environment.
`credentials.FromEnvironment("SELECTEL_")` creates a lazy source; it reads only
that prefix on Lookup. The [SDK README](../../../../README.md)
lists its recognized fields. AccountID in Config is still explicit; credentials
do not choose an active account/project. FromProfile(path,name) and
FromKeyring(store,entry) are alternatives when that source is configured.

The following program reads project count from an account view. It requires
explicit SELECTEL_ACCOUNT_ID and valid access for the same account; it performs
real cloud reads only when executed with that source. It prints count and
completeness, not raw cloud payloads.

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    sdk "github.com/festy23/selectel-sdk"
    "github.com/festy23/selectel-sdk/credentials"
    "github.com/festy23/selectel-sdk/pagination"
    "github.com/festy23/selectel-sdk/projects"
)

func main() {
    source, err := credentials.FromEnvironment("SELECTEL_")
    if err != nil { panic(err) }
    client, err := sdk.New(sdk.Config{
        AccountID: os.Getenv("SELECTEL_ACCOUNT_ID"),
        Credentials: source,
    })
    if err != nil { panic(err) }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    result, err := client.Account().Projects().Collect(ctx,
        projects.ListOptions{}, pagination.Options{MaxItems: 100, MaxPages: 10})
    fmt.Printf("count=%d complete=%t stop=%s\n",
        len(result.Items), result.Complete, result.StopReason)
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
```

For regional methods, create `client.Project(projectID)`, then
`project.Location(region,pool)` and the selected service view. Each factory
returns an error that must be handled before use. Do not copy example IDs or
locations into real actions; derive the target from the task and cloud reads.

Read the chosen service source for its exact request/result types. Keep source
generation explicit; MKS v2, Registry v1/v2, and Nova v2.1 use distinct bindings.
Persist only SDK receipts and safe metadata when recovery is required. Keep
protected cursors private; explicit Cursor.Export discloses a token for storage.
