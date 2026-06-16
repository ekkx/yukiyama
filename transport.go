package yukiyama

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// ctxExtraQueryKey is the context key under which facade methods can stash
// additional query parameters that the auto-generated gen builders do not
// expose setters for. The transport reads this map during injectAuth and
// merges entries into req.URL.
//
// This mechanism is currently unused by any facade — the gen builder's
// typed setters cover every operation we ship. It is kept in place for
// future extensions (e.g. a new endpoint whose schema cannot be expressed
// through a gen setter). withExtraQuery and extraQueryFromContext are
// intentionally exported only within-package.
type ctxExtraQueryKey struct{}

// withExtraQuery returns a context that, when used to drive a gen builder
// Execute(), causes authTransport to add the supplied key=value pairs to the
// outgoing request URL. Internal use only; retained as an escape hatch for
// endpoints whose schema cannot be expressed through a gen setter.
func withExtraQuery(ctx context.Context, kv map[string]string) context.Context {
	if len(kv) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxExtraQueryKey{}, kv)
}

// extraQueryFromContext extracts the extra-query map planted by
// withExtraQuery, or returns nil. See withExtraQuery for status.
func extraQueryFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(ctxExtraQueryKey{}).(map[string]string)
	return v
}

// authTransport is an http.RoundTripper that
//  1. injects user_id / token / version onto the URL query of every outbound
//     request (unless the path is in authExemptPaths),
//  2. detects {status:false, error_code:103} ("session expired") in the response
//     body and transparently re-logins + retries the original request once.
//
// Recursion is avoided because the login endpoint itself is exempt from
// injection (see authExemptPaths), so the Login flow does not recursively
// re-enter the auth path inside its own RoundTrip.
type authTransport struct {
	base       http.RoundTripper
	client     *Client
	maxRetries int
}

// authExemptPaths are endpoints that must NOT have user_id/token injected,
// either because they ARE the login (and thus have no session yet) or because
// they predate the session model. Path matches are done against
// req.URL.Path AND against a "suffix after /api" form so that custom apiBase
// values that include the /api prefix still match.
var authExemptPaths = map[string]bool{
	"/user/mail_auth":        true,
	"/user/one_time_pw_auth": true,
	"/user/regist":           true,
	"/user/login":            true,
}

// isAuthExempt returns true when the request's path matches a known login /
// pre-auth endpoint. It tolerates apiBase variants that include or exclude the
// "/api" suffix.
func isAuthExempt(path string) bool {
	if authExemptPaths[path] {
		return true
	}
	// Strip a leading "/api" prefix and retry (apiBase may include /api).
	const apiPrefix = "/api"
	if len(path) > len(apiPrefix) && path[:len(apiPrefix)] == apiPrefix {
		return authExemptPaths[path[len(apiPrefix):]]
	}
	return false
}

// RoundTrip implements http.RoundTripper.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.setDefaultUserAgent(req)
	exempt := isAuthExempt(req.URL.Path)
	if !exempt {
		if err := t.injectAuth(req); err != nil {
			return nil, err
		}
	}
	reqBody, err := captureRequestBody(req)
	if err != nil {
		return nil, err
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	bodyBytes, err := readAndReplayBody(resp)
	if err != nil {
		return resp, err
	}

	if exempt || t.maxRetries <= 0 {
		return resp, nil
	}
	return t.maybeReloginRetry(req, reqBody, resp, bodyBytes)
}

// setDefaultUserAgent installs the configured UA when the caller didn't set one.
func (t *authTransport) setDefaultUserAgent(req *http.Request) {
	if req.Header.Get("User-Agent") != "" {
		return
	}
	if t.client == nil || t.client.cfg == nil || t.client.cfg.userAgent == "" {
		return
	}
	req.Header.Set("User-Agent", t.client.cfg.userAgent)
}

// captureRequestBody reads req.Body so it can be replayed on retry, then
// reattaches a fresh reader. POST bodies are rare on this API (almost
// everything is GET) but the path supports them. Returns nil when the body
// is absent or the caller already provided GetBody.
func captureRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.GetBody != nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// readAndReplayBody drains resp.Body and replaces it with a re-readable
// in-memory reader so the SDK decoder downstream can still read it. The
// returned bytes are used here to look for the {status:false, error_code:103}
// envelope.
func readAndReplayBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, err
}

// maybeReloginRetry inspects bodyBytes for a session-expired envelope and, if
// it sees one, clears the cached session, runs Login, rebuilds the original
// request with fresh credentials, and retries through the base transport
// exactly once. Returns the original response unchanged when no retry is
// warranted (or when relogin fails).
func (t *authTransport) maybeReloginRetry(req *http.Request, reqBody []byte, resp *http.Response, bodyBytes []byte) (*http.Response, error) {
	apiErr := parseAPIErrorFromBody(resp.StatusCode, bodyBytes)
	if apiErr == nil || !apiErr.IsAuthExpired() {
		return resp, nil
	}

	t.client.log().Warn("yukiyama: error_code=103 detected, attempting re-login + retry", "path", req.URL.Path)
	t.client.session.Clear()
	if loginErr := t.client.Login(req.Context()); loginErr != nil {
		// Re-login failed: surface the original 103 envelope so the caller
		// still sees a decodable response (IsAuthExpired() will report true).
		t.client.log().Warn("yukiyama: re-login failed, returning original 103 response", "err", loginErr)
		return resp, nil
	}

	retryReq, cloneErr := cloneRequest(req, reqBody)
	if cloneErr != nil {
		return resp, cloneErr
	}
	stripStaleAuth(retryReq)
	if err := t.injectAuth(retryReq); err != nil {
		return resp, err
	}
	// Route the retry through the base transport directly, not back through
	// RoundTrip, so a second 103 surfaces to the caller naturally instead of
	// looping.
	return t.base.RoundTrip(retryReq)
}

// stripStaleAuth removes user_id and token from the URL of a retry request so
// the next injectAuth pass (which uses "fill if missing" semantics) can
// re-populate them from the post-relogin session. `version` is left untouched
// because a caller-pinned content-schema selector applies equally on retry.
func stripStaleAuth(req *http.Request) {
	q := req.URL.Query()
	if q.Get("user_id") == "" && q.Get("token") == "" {
		return
	}
	q.Del("user_id")
	q.Del("token")
	req.URL.RawQuery = q.Encode()
}

// injectAuth augments req.URL.RawQuery with user_id, token, and version. If no
// session exists and autoLogin is enabled, Login() is invoked first.
func (t *authTransport) injectAuth(req *http.Request) error {
	userID, token, ok := t.client.session.Snapshot()
	if !ok {
		if !t.client.cfg.autoLogin {
			return errors.New("yukiyama: session not authenticated; call client.Login(ctx) first or enable WithAutoLogin(true)")
		}
		if err := t.client.Login(req.Context()); err != nil {
			return err
		}
		userID, token, ok = t.client.session.Snapshot()
		if !ok {
			return errors.New("yukiyama: Login succeeded but session is still empty (bug)")
		}
	}
	// user_id / token / version are uniformly "fill if missing" — any value
	// the caller explicitly set on the gen builder wins. This lets a facade
	// pre-populate a non-session value such as CheckUserNameAvailable's
	// `user_id=<username string>` without the transport stomping it. For
	// ordinary endpoints nothing is set, so we fill from the live session.
	// The 103 retry path wipes the stale auth triple before re-entering
	// injectAuth so the post-relogin session lands.
	q := req.URL.Query()
	if q.Get("user_id") == "" {
		q.Set("user_id", strconv.Itoa(int(userID)))
	}
	if q.Get("token") == "" {
		q.Set("token", token)
	}
	// `version` is more nuanced. Most endpoints want the transport-injected
	// APIVersionName (the app version), but a handful want a content-schema
	// selector instead (getHomeData "5", getUnreadCount "2", getUserProfile
	// "3", listDistributionNotifications "2"). Facades for those set
	// `.Version(...)` on the gen builder; we must only fill in APIVersionName
	// as a fallback when nothing explicit was set.
	if q.Get("version") == "" {
		q.Set("version", APIVersionName)
	}
	// Merge per-call extras (e.g. skiarea_id) planted by facade.go via the
	// request's context. We do this AFTER the user_id/token/version overwrite
	// so a facade cannot accidentally clobber the session identity, but
	// BEFORE Encode() so the additions land in the same RawQuery write.
	if extras := extraQueryFromContext(req.Context()); len(extras) > 0 {
		for k, v := range extras {
			// Defensive: never let extras overwrite the auth triple. If a
			// caller passes user_id here, ignore it silently.
			switch k {
			case "user_id", "token", "version":
				continue
			}
			q.Set(k, v)
		}
	}
	req.URL.RawQuery = q.Encode()
	return nil
}

// cloneRequest deep-copies req for a retry. The body (already drained earlier)
// is re-wrapped as a fresh io.Reader so the retry can send it again.
func cloneRequest(req *http.Request, body []byte) (*http.Request, error) {
	r2 := req.Clone(req.Context())
	if len(body) > 0 {
		r2.Body = io.NopCloser(bytes.NewReader(body))
		r2.ContentLength = int64(len(body))
		r2.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	} else {
		r2.Body = nil
		r2.GetBody = nil
	}
	return r2, nil
}
