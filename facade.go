package yukiyama

import (
	"context"
	"errors"
	"fmt"

	gen "github.com/ekkx/yukiyama/gen"
)

// This file holds the shared client-level helpers used by every service-grouped
// facade:
//
//   - ensureSession: "make sure (user_id, token) is usable" guard that every
//     service method calls at the top of its body.
//   - statusOrError: "fold a CommonResponse + transport error into a single
//     facade-side error" helper for fire-and-forget mutations.
//
// Per-domain operations (including the full session-lifecycle trio Login,
// Logout, Withdraw) live on client.User, client.Checkin, client.Common,
// client.Skiarea, client.Ranking, client.Safety. Top-level *Client only
// carries session-state inspection (IsAuthenticated, CurrentUserID,
// CurrentToken, SetSession, SessionStore) and the Gen() escape hatch.

// ensureSession guarantees that c has a usable (user_id, token). If autoLogin
// is enabled it triggers User.Login when needed; otherwise it returns an
// actionable error pointing the caller at client.User.Login(ctx).
func (c *Client) ensureSession(ctx context.Context) error {
	if c.session.IsAuthenticated() {
		return nil
	}
	if c.cfg == nil || !c.cfg.autoLogin {
		return errors.New("yukiyama: not authenticated; call client.User.Login(ctx) first or enable WithAutoLogin(true)")
	}
	return c.User.Login(ctx)
}

// statusOrError converts a CommonResponse + transport error into the canonical
// facade-side error for fire-and-forget mutations. err wins over status=false.
func statusOrError(res *gen.CommonResponse, err error, opName, cmd string) error {
	if err != nil {
		return err
	}
	if res != nil && !res.GetStatus() {
		return fmt.Errorf("yukiyama: %s cmd=%q failed: %s", opName, cmd, res.GetError())
	}
	return nil
}
