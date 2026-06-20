package yukiyama

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/ekkx/yukiyama/gen"
)

// Default endpoints and headers. Override via WithAPIBase / WithUserAgent.
const (
	defaultAPIBase   = "https://admin.yukiyama.biz/api"
	defaultUserAgent = "Mozilla/5.0 (Linux; Android 14; SM-S908N Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0.0.0 Mobile Safari/537.36"
)

// Client is the entry point for the yukiyama SDK. Domain operations are
// grouped onto service-typed accessors (client.User, client.Checkin,
// client.Common, client.Skiarea, client.Ranking, client.Safety). The full
// session-lifecycle trio (Login, Logout, Withdraw) lives on client.User as
// a coherent group; session-state inspection (IsAuthenticated,
// CurrentUserID, CurrentToken, SetSession) stays on the client itself
// because it never touches the network. For endpoints not yet covered by
// a service, Gen() returns the raw generated *gen.APIClient as an escape
// hatch:
//
//	client.User.Login(ctx)                          // session lifecycle
//	client.User.Logout()                            // session lifecycle (local)
//	client.Skiarea.SearchSkiareasByLocation(ctx, lat, lng, yukiyama.SearchSkiareasByLocationOptions{})
//	client.Gen().CommonAPI.SomeNewOp(ctx).Execute() // escape hatch
//
// Authentication state (user_id, token) is injected onto every request by
// the underlying transport, so callers should not set those query params
// manually.
type Client struct {
	// api is the underlying generated API client. Used by the SDK internally
	// and exposed via Gen() as an escape hatch for callers that want to
	// reach the whole generated surface in one place.
	api *gen.APIClient

	// Service-grouped facades. Each one groups operations under a single
	// wire-path prefix and owns the wire-quirk corrections for that group.
	Checkin *CheckinService
	Common  *CommonService
	Ranking *RankingService
	Safety  *SafetyService
	Skiarea *SkiareaService
	User    *UserService

	cfg     *config
	session *Session

	// loginMu serializes concurrent Login() invocations so that a burst of
	// goroutines arriving with no session triggers only one login round-trip.
	loginMu sync.Mutex
}

// NewClient builds a Client.
//
// Default behavior:
//   - apiBase = https://admin.yukiyama.biz/api
//   - userAgent = a typical Android WebView UA (matches the official app)
//   - autoLogin = true (the first authenticated call calls Login() if needed)
//   - 1 transparent re-login + retry on error_code 103 ("session expired")
//
// Credentials are NOT read from any environment by the SDK; pass them via
// WithCredentials. Reading them from the host environment (.env, secret
// manager, etc.) is the caller's responsibility.
//
// Customize via WithCredentials, WithAPIBase, WithUserAgent, WithHTTPClient,
// WithLogger, WithSessionStore, or WithAutoLogin.
func NewClient(opts ...Option) (*Client, error) {
	cfg := &config{
		apiBase:         defaultAPIBase,
		userAgent:       defaultUserAgent,
		autoLogin:       true,
		maxRetries:      1,
		logger:          noopLogger{},
		sessionStore:    NoopSessionStore{},
		autoLoadSession: true,
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.sessionStore == nil {
		cfg.sessionStore = NoopSessionStore{}
	}

	// Normalize: API base must not have a trailing slash (the generated
	// SDK concatenates "/user/get" etc.).
	cfg.apiBase = strings.TrimRight(cfg.apiBase, "/")

	c := &Client{
		cfg:     cfg,
		session: &Session{},
	}

	// Wrap whatever Transport the user supplied (or http.DefaultTransport)
	// with authTransport so credentials inject and 103 retries happen.
	var baseTransport http.RoundTripper = http.DefaultTransport
	if cfg.httpClient != nil && cfg.httpClient.Transport != nil {
		baseTransport = cfg.httpClient.Transport
	}
	wrappedHTTP := &http.Client{
		Transport: &authTransport{
			base:       baseTransport,
			client:     c,
			maxRetries: cfg.maxRetries,
		},
	}
	if cfg.httpClient != nil {
		// Preserve user-set timeouts, cookie jars, etc., but force our Transport.
		wrappedHTTP.Timeout = cfg.httpClient.Timeout
		wrappedHTTP.CheckRedirect = cfg.httpClient.CheckRedirect
		wrappedHTTP.Jar = cfg.httpClient.Jar
	}

	genCfg := gen.NewConfiguration()
	genCfg.Servers = gen.ServerConfigurations{
		{URL: cfg.apiBase, Description: "primary"},
	}
	genCfg.UserAgent = cfg.userAgent
	genCfg.HTTPClient = wrappedHTTP

	c.api = gen.NewAPIClient(genCfg)

	// Wire service-grouped facades. Each service holds a back-pointer to the
	// Client so methods reach the gen layer via c.api.XAPI and inherit the
	// session / autoLogin / autoRetry behavior on every call.
	c.Checkin = &CheckinService{c: c}
	c.Common = &CommonService{c: c}
	c.Ranking = &RankingService{c: c}
	c.Safety = &SafetyService{c: c}
	c.Skiarea = &SkiareaService{c: c}
	c.User = &UserService{c: c}

	// Hydrate session from the configured SessionStore. Failures are logged
	// and swallowed: a missing or unreadable store should not block
	// construction — the caller can still Login() explicitly.
	if cfg.autoLoadSession {
		if persisted, err := cfg.sessionStore.Load(context.Background()); err != nil {
			c.log().Warn("yukiyama: SessionStore.Load failed", "err", err)
		} else if persisted != nil && persisted.UserID != 0 && persisted.Token != "" {
			c.session.Set(persisted.UserID, persisted.Token)
			c.log().Debug("yukiyama: session restored from store", "user_id", persisted.UserID)
		}
	}

	return c, nil
}

// IsAuthenticated reports whether the client currently holds a usable session
// (either set by Login, restored from a SessionStore, or installed via
// SetSession). Useful for callers that want to skip an explicit Login when a
// session was hydrated by WithSessionStore.
func (c *Client) IsAuthenticated() bool {
	return c.session.IsAuthenticated()
}

// Gen returns the underlying *gen.APIClient as an escape hatch for endpoints
// the service-grouped accessors do not yet wrap. Typical use:
//
//	res, _, err := client.Gen().CommonAPI.SomeNewOp(ctx).Execute()
//
// Prefer the service-grouped accessor (client.User, client.Checkin, ...)
// when one exists — those wrap wire-naming quirks and content-schema version
// pinning that callers would otherwise have to remember endpoint-by-endpoint.
//
// Auth (user_id / token / version) is injected by the underlying transport on
// every path, so calls through Gen() are authenticated identically; only the
// ensureSession() pre-check and the service-level rename / Options ergonomics
// are skipped.
func (c *Client) Gen() *gen.APIClient {
	return c.api
}

// SessionStore returns the configured SessionStore. Exposed so tests and
// advanced callers can inspect or manually Save/Load/Clear out-of-band.
func (c *Client) SessionStore() SessionStore {
	if c == nil || c.cfg == nil || c.cfg.sessionStore == nil {
		return NoopSessionStore{}
	}
	return c.cfg.sessionStore
}

// log returns the configured Logger, falling back to a no-op if not set.
func (c *Client) log() Logger {
	if c.cfg != nil && c.cfg.logger != nil {
		return c.cfg.logger
	}
	return noopLogger{}
}
