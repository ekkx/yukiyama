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

// Client is the high-level yukiyama API client. It embeds each
// auto-generated service so every operation is reachable as a flat method
// on the client (no service-tag prefix required):
//
//	client.GetMyProfile(ctx)                   // handwritten facade
//	client.GetMaster(ctx).Execute()            // generated builder, flat
//	client.ListMyCoupons(ctx).Execute()        // generated builder, flat
//
// When the same name exists on both a handwritten facade and an embedded
// gen service, Go's method-resolution rules pick the facade (the outer
// type shadows the promoted method). Reach for the gen builder explicitly
// via the embedded field name (e.g. client.CommonAPIService.GetHomeData)
// or via Gen() for the full *gen.APIClient escape hatch.
//
// Authentication state (user_id, token) is injected onto every request by
// the underlying authTransport, so callers should not set those query
// params manually.
type Client struct {
	// Flat-promoted generated services. Methods on each are reachable
	// directly on *Client unless shadowed by a handwritten facade.
	*gen.CheckinAPIService
	*gen.CommonAPIService
	*gen.RankingAPIService
	*gen.SafetyAPIService
	*gen.SkiareaAPIService
	*gen.UserAPIService

	// api is the underlying gen.APIClient. Used by the SDK internally and
	// exposed via Gen() as an escape hatch for callers that want to reach
	// the whole gen surface in one place.
	api *gen.APIClient

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
	// Wire each generated service onto the embed slot so methods promote
	// directly onto *Client. The gen layer shares one internal `service`
	// struct across every XxxAPIService, so these assignments are pointer
	// aliasing into the same underlying state — no extra allocation.
	c.CheckinAPIService = c.api.CheckinAPI
	c.CommonAPIService = c.api.CommonAPI
	c.RankingAPIService = c.api.RankingAPI
	c.SafetyAPIService = c.api.SafetyAPI
	c.SkiareaAPIService = c.api.SkiareaAPI
	c.UserAPIService = c.api.UserAPI

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

// Gen returns the underlying *gen.APIClient. Day-to-day callers do not need
// this — every operation is already promoted onto *Client via embedded
// service pointers — but it is useful when you need the gen client as a
// value (e.g. passing to a helper) or want to reach a specific service
// when a facade has shadowed its name (see CommonAPIService et al. on the
// embed slot, or call client.Gen().CommonAPI.GetHomeData(ctx) here).
//
// Auth (user_id / token / version) is injected by authTransport on every
// path, so calls through Gen() are authenticated identically; only the
// ensureSession() pre-check and any facade-level rename / Options
// ergonomics are skipped.
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
