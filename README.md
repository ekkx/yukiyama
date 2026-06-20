<p align="center">
  <img src="./assets/logo.webp" width="128" alt="yukiyama">
</p>

<h1 align="center">yukiyama</h1>

<p align="center">
  Go SDK for the yukiyama API.
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/ekkx/yukiyama"><img src="https://pkg.go.dev/badge/github.com/ekkx/yukiyama.svg" alt="Go Reference"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

<p align="center">
  <img src="./assets/bg.png" width="100%" alt="">
</p>

## Install

```bash
go get github.com/ekkx/yukiyama
```

## Quick start

```go
package main

import (
    "context"
    "log"

    "github.com/ekkx/yukiyama"
)

func main() {
    ctx := context.Background()

    client, err := yukiyama.NewClient(
        yukiyama.WithCredentials("you@example.com", "password"),
        yukiyama.WithSessionStore(yukiyama.NewFileSessionStore("")),
    )
    if err != nil {
        log.Fatal(err)
    }

    // FileSessionStore hydrates from disk; second run skips Login.
    if !client.IsAuthenticated() {
        if err := client.User.Login(ctx); err != nil {
            log.Fatal(err)
        }
    }

    // Read: own profile.
    profile, err := client.User.GetMyUserProfile(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("hello, %s", profile.GetProfile().GetUserName())

    // Read: ski areas near a point.
    areas, err := client.Skiarea.SearchSkiareasByLocation(ctx, 36.7, 138.0, yukiyama.SearchSkiareasByLocationOptions{})
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("areas: status=%v", areas.GetStatus())

    // Mutation: like a checkin.
    if err := client.Checkin.LikeCheckin(ctx, 12345); err != nil {
        log.Fatal(err)
    }
}
```

## Highlights

| | |
| --- | --- |
| **Service-grouped API** | Operations live on `client.User`, `client.Checkin`, `client.Common`, `client.Skiarea`, `client.Ranking`, `client.Safety`. The session-lifecycle trio (`User.Login`, `User.Logout`, `User.Withdraw`) groups with the other `/user/*` ops; in-memory state inspection (`IsAuthenticated`, `CurrentUserID`, `CurrentToken`, `SetSession`) stays on `*Client`. |
| **Session lifecycle** | `client.User.Login` / `client.User.Logout` / `client.User.Withdraw` with the underlying `(user_id, token)` cached automatically. |
| **Pluggable persistence** | Built-in `FileSessionStore` (atomic write, mode `0600`), or implement the `SessionStore` interface for Redis / Keychain / etc. |
| **Transparent re-login** | `error_code: 103` is detected, the cached session is cleared, `Login` runs again, and the original request is retried once. |
| **Typed Options** | Endpoints with many optional filters take an `Options` struct of pointer fields so omission and the empty string never get confused. |
| **Wire corrections built in** | Wire-naming quirks (caller/target reversals, content-schema `version` selectors, username-as-`user_id`, etc.) are handled by the service methods so callers don't repeat them. |

## Low-level access

For endpoints not yet wrapped by a service method, drop down to the
generated client via `Gen()`:

```go
res, _, err := client.Gen().CommonAPI.SomeNewOp(ctx).Execute()
```

`user_id` / `token` / `version` are still injected by the transport on
this path; only the service-level rename and Options ergonomics are
skipped.

## Documentation

Full API reference: [pkg.go.dev/github.com/ekkx/yukiyama](https://pkg.go.dev/github.com/ekkx/yukiyama).

## Status

Pre-1.0. The Go module pins to a single upstream API version (see
[`yukiyama.APIVersionName`](https://pkg.go.dev/github.com/ekkx/yukiyama#pkg-constants)).
Breaking changes are possible as either the upstream or the SDK firms up.

## Disclaimer

This SDK is published for research and educational purposes only. The
authors and contributors make no warranty and accept no liability for any
use of this software.

Distributed under the [MIT License](./LICENSE).
