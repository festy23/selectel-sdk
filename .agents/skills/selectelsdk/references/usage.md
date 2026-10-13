# Go execution and explicit access

## Install and locate the SDK

For SDK plus a globally discoverable skill and personal profile, use the
[one-command installer](../../../../README.md#установить-sdk-и-скилл-одной-командой).
When `installation.json` exists beside the installed SKILL.md, resolve the SDK
root, Go executable and selected profile from it before using the examples.
That metadata is local setup input, not an automatic credential loader in SDK
core. The installer rebases repository references to its installed SDK revision.

Use a Go toolchain compatible with the repository's `go.mod`. For agent work
with the SDK source, skill and capability catalog available together:

```sh
git clone https://github.com/festy23/selectel-sdk.git
cd selectel-sdk
go mod download
```

Reuse an existing checkout when available. Resolve its absolute path and run
one-off programs from that module directory; imports then use the local SDK.
Keep their source in a temporary directory outside Git:

```sh
go run /absolute/path/to/program.go
```

For an existing Go application, run from that application's module directory:

```sh
go get github.com/festy23/selectel-sdk@main
```

This records a version in the application's `go.mod` and `go.sum`; later upstream
commits do not update it automatically. Select an explicit tag or commit when
the task requires a particular revision. Read the capability catalog and source
of that same SDK revision when choosing methods.

The agent entrypoint is `.agents/skills/selectelsdk/SKILL.md` in the SDK checkout,
also routed by its `AGENTS.md`. Point the agent to that file when working from
another repository. Its references depend on the SDK repository layout; retain
access to the checkout rather than copying only `SKILL.md`. Go dependency
installation with `go get` does not register this skill or install a cloud CLI.

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
    "github.com/festy23/selectel-sdk/types"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintf(os.Stderr, "SDK error: %s\n", types.ErrorCode(err))
        os.Exit(1)
    }
}

func run() error {
    source, err := credentials.FromEnvironment("SELECTEL_")
    if err != nil { return err }
    client, err := sdk.New(sdk.Config{
        AccountID: os.Getenv("SELECTEL_ACCOUNT_ID"),
        Credentials: source,
    })
    if err != nil { return err }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    result, err := client.Account().Projects().Collect(ctx,
        projects.ListOptions{}, pagination.Options{MaxItems: 100, MaxPages: 10})
    fmt.Printf("count=%d complete=%t stop=%s\n",
        len(result.Items), result.Complete, result.StopReason)
    return err
}
```

### Use a named profile

Reuse the user's saved profile by its explicit file path and name. For a new
profile, the JSON root maps names directly to credential records:

```json
{
  "my-project": {
    "account_id": "ACCOUNT_NUMBER",
    "username": "SERVICE_USER_NAME",
    "password": "PASSWORD_FROM_LOCAL_SECRET_STORE"
  }
}
```

These are placeholders, not usable credentials. Save real material locally
outside Git, with owner-only file access (`chmod 600`) and a private parent
directory. `username` is the IAM service user's name, not its UID; `account_id`
is the account number, not a user or project ID. Available fields and other
credential schemes are described in the [SDK README](../../../../README.md#другие-способы-доступа-токены-логи-и-метрики).

For the bounded read program above, set these non-secret inputs to your values:

```sh
export SDK_PROFILE_PATH="$HOME/.config/selectelsdk/profiles.json"
export SDK_PROFILE_NAME='my-project'
export SELECTEL_ACCOUNT_ID='ACCOUNT_NUMBER'
```

Replace its `credentials.FromEnvironment("SELECTEL_")` line with:

```go
source, err := credentials.FromProfile(
    os.Getenv("SDK_PROFILE_PATH"), os.Getenv("SDK_PROFILE_NAME"))
```

Keep the following error check and explicit Config.AccountID. The two `SDK_`
variables are inputs to this example, not automatically discovered SDK settings.
FromProfile reads the selected record on Lookup; it does not merge environment
credentials or choose an active profile. The first service call acquires the
required IAM token lazily.

The account read example requires account-level permission. A record with
`project_id` binds access to that exact project and cannot serve account Billing
or Projects methods. For project reads, use an authorized project view and the
relevant service method. A 403 does not justify expanding the user's roles.

## Select the service and interpret failures

For regional methods, create `client.Project(projectID)`, then
`project.Location(region,pool)` and the selected service view. Each factory
returns an error that must be handled before use. Do not copy example IDs or
locations into real actions; derive the target from the task and cloud reads.

Read the chosen service source for its exact request/result types. Keep source
generation explicit; MKS v2, Registry v1/v2, and Nova v2.1 use distinct bindings.
Persist only SDK receipts and safe metadata when recovery is required. Keep
protected cursors private; explicit Cursor.Export discloses a token for storage.

For failed first calls, follow the [README troubleshooting table](../../../../README.md#если-первый-запрос-не-работает).
Report error code, HTTP status, operation ID and request IDs when available;
keep credential values and raw provider responses private. A diagnostic call
with modified request formatting or transport must be reported separately from
an unmodified SDK call.
