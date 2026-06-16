// Package yukiyama is a Go SDK for the yukiyama mobile app API.
//
// It wraps the auto-generated low-level client in yukiyama/gen with a
// session-aware transport that:
//
//   - Injects user_id, token, and version query parameters onto every request.
//   - Transparently re-logins and retries on error_code 103
//     ("ログインセッションが終了しました", single-session expiry).
//   - Accepts credentials via WithCredentials at construction time.
//     The SDK never reads from the host environment — sourcing creds from
//     .env / secret manager / etc. is the caller's responsibility.
//   - Optionally persists the session to disk (or any custom SessionStore) so
//     a restarted process can skip the Login round-trip entirely.
//
// # Quick start (six lines, with disk persistence and a facade call)
//
//	store := yukiyama.NewFileSessionStore("") // default ~/.config/yukiyama/session.json
//	client, _ := yukiyama.NewClient(yukiyama.WithSessionStore(store))
//	if !client.IsAuthenticated() {
//	    _ = client.Login(ctx) // first run only; subsequent runs hydrate from disk
//	}
//	profile, err := client.GetMyProfile(ctx)
//
// # Low-level access
//
// Every generated operation is promoted directly onto *Client via embedded
// service pointers, so endpoints with no handwritten facade are flat too:
//
//	res, _, err := client.GetMaster(ctx).Execute()
//	res, _, err := client.ListMyCoupons(ctx).Execute()
//	res, _, err := client.GetUnreadCount(ctx).Execute()
//
// When a facade and a generated operation share a name (e.g. GetHomeData),
// Go's method-resolution rules pick the facade. Reach for the unwrapped
// gen builder via the embedded field name if you need it:
//
//	res, _, err := client.CommonAPIService.GetHomeData(ctx).Execute()
//
// Or use Gen() for the whole *gen.APIClient as an escape hatch.
//
// Login is also called lazily on the first authenticated request when
// WithAutoLogin(true) (the default) is in effect.
//
// # Facades
//
// Handwritten facade methods exist where they add value over the raw gen
// builder: wire-naming corrections, caller/target reversals, content-schema
// version pinning, SessionStore lifecycle, and Options bundling. See each
// method's godoc for the specific wire-shape caveat it handles.
//
// Auth-triple injection (user_id, token, version) on the transport is
// "fill if missing" — any value a caller or facade sets on the gen builder
// wins. The 103 → re-login retry path wipes stale auth before reinjection
// so the post-relogin session lands on retries.
//
// # Session persistence
//
// Pass a SessionStore via WithSessionStore() and NewClient hydrates the
// in-memory session at construction time. Login() Save()s the new session,
// Logout() Clear()s it. Built-ins:
//
//   - NoopSessionStore (default; no persistence).
//   - FileSessionStore (JSON file, mode 0600, atomic temp+rename writes).
//
// Implement the SessionStore interface for keychain, Redis, or any other
// backend. Store failures are logged via the configured Logger but never
// abort the in-memory auth path.
//
// # Error handling
//
// Wire failures carry a {status:false, error:string, error_code?:int}
// envelope. The SDK exposes APIError and the convenience predicates
// yukiyama.CodeOf(err) and yukiyama.IsAuthExpired(err).
//
// # Stability
//
// Pre-1.0. The API surface tracks the generated gen package and the upstream
// at API version 10.3.3; breaking changes are possible as either firms up.
package yukiyama
