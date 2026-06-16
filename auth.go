package yukiyama

import (
	"context"
	"errors"
	"strconv"
)

// Login authenticates with the credentials configured via WithCredentials().
// The resulting (user_id, token) pair is cached in c.session for all
// subsequent requests.
//
// Login is safe to call concurrently: the loginMu mutex serializes concurrent
// callers, and the double-check pattern means a second goroutine arriving
// after the first has succeeded becomes a no-op.
//
// Login is also invoked automatically by authTransport when:
//   - No session exists and WithAutoLogin(true) (the default) is set.
//   - A response is detected as {status:false, error_code:103} ("session
//     expired"); the transport clears the cache and re-logins exactly once.
func (c *Client) Login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	// Double-check: another goroutine may have authenticated while we waited.
	if c.session.IsAuthenticated() {
		return nil
	}

	if c.cfg.mail == "" || c.cfg.password == "" {
		return errors.New("yukiyama: no credentials; pass them via WithCredentials() at construction time")
	}

	// Note: /user/login is in authExemptPaths, so the RoundTrip below will
	// NOT recursively invoke injectAuth → Login. This is the invariant that
	// breaks the recursion cycle.
	res, _, err := c.api.UserAPI.LoginWithEmail(ctx).
		Mail(c.cfg.mail).
		Pw(c.cfg.password).
		Platform("android").
		Execute()
	if err != nil {
		return err
	}

	// LoginResponse wire shape: ALL fields are string ("status":"1", "id":"12345").
	if res.GetStatus() != "1" {
		return &APIError{
			StatusCode: 200, // login transport returned a 200-with-status:"0" envelope
			Status:     false,
			Message:    res.GetError(),
			// LoginResponse has no error_code field; leave as 0.
		}
	}
	userID64, parseErr := strconv.ParseInt(res.GetId(), 10, 32)
	if parseErr != nil {
		return errors.New("yukiyama: login succeeded but returned non-numeric id: " + res.GetId())
	}
	c.session.Set(int32(userID64), res.GetToken())
	c.log().Debug("yukiyama: login ok", "user_id", userID64)

	// Persist to the configured SessionStore (NoopSessionStore by default).
	// Save failures are non-fatal — Login itself succeeded, the in-memory
	// session is valid, and the caller should not be blocked by a flaky
	// store.
	if store := c.cfg.sessionStore; store != nil {
		if err := store.Save(ctx, &PersistedSession{
			UserID: int32(userID64),
			Token:  res.GetToken(),
		}); err != nil {
			c.log().Warn("yukiyama: SessionStore.Save failed", "err", err)
		}
	}
	return nil
}

// Logout clears the cached session and, if a SessionStore is configured,
// asks the store to drop any persisted copy.
//
// Note: this is a CLIENT-SIDE clear only; yukiyama follows a single-session
// model where logging in elsewhere invalidates the prior token. There is no
// /user/logout endpoint.
//
// Store.Clear failures are logged via the configured Logger and otherwise
// swallowed — the in-memory clear always succeeds, which is the load-bearing
// invariant callers care about.
func (c *Client) Logout() {
	c.session.Clear()
	if c.cfg != nil && c.cfg.sessionStore != nil {
		if err := c.cfg.sessionStore.Clear(context.Background()); err != nil {
			c.log().Warn("yukiyama: SessionStore.Clear failed", "err", err)
		}
	}
}

// CurrentUserID returns the authenticated user_id, or 0 when no session is
// active. Useful for callers that need to fill `id` query params (e.g.
// GetUserProfile().Id(client.CurrentUserID())).
func (c *Client) CurrentUserID() int32 {
	id, _, _ := c.session.Snapshot()
	return id
}

// CurrentToken returns the cached session token, or "" when not authenticated.
// Exposed mainly for diagnostics and for callers that want to persist sessions
// across process restarts.
func (c *Client) CurrentToken() string {
	_, tok, _ := c.session.Snapshot()
	return tok
}

// SetSession installs an externally obtained (user_id, token) pair into the
// client. Useful for restoring a persisted session without going through
// Login() again. Subsequent calls will use this identity; if it has expired
// the transport will detect 103 and re-login (assuming credentials are also
// configured).
func (c *Client) SetSession(userID int32, token string) {
	c.session.Set(userID, token)
}
