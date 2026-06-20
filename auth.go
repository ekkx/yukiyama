package yukiyama

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
