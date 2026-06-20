package yukiyama

import (
	"net/http"
)

// Option configures a Client at construction time. Apply via NewClient.
type Option func(*config)

// config is the internal config struct populated by Option functions inside
// NewClient. It is intentionally unexported; the public API is the WithXxx
// Option set.
type config struct {
	apiBase         string // default defaultAPIBase
	userAgent       string // default defaultUserAgent
	mail            string // login credentials (set via WithCredentials)
	password        string // login credentials (set via WithCredentials)
	httpClient      *http.Client
	logger          Logger
	autoLogin       bool
	maxRetries      int
	sessionStore    SessionStore // default NoopSessionStore{}
	autoLoadSession bool         // default true
}

// WithCredentials sets the mail/password used by Login (and the autoLogin path
// in authTransport). The SDK does not read credentials from the environment;
// callers are expected to source them from .env / secret manager / etc. and
// pass them here.
func WithCredentials(mail, password string) Option {
	return func(c *config) {
		c.mail = mail
		c.password = password
	}
}

// WithAPIBase overrides the API base URL. A trailing slash is trimmed.
// Default: "https://admin.yukiyama.biz/api".
func WithAPIBase(base string) Option {
	return func(c *config) {
		c.apiBase = base
	}
}

// WithUserAgent overrides the default User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *config) {
		c.userAgent = ua
	}
}

// WithHTTPClient lets callers supply their own *http.Client. The supplied
// client's Transport (if any) is wrapped by authTransport so that auth
// injection / 103 retry still apply. Use this for custom timeouts, proxies,
// or a recording RoundTripper in tests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) {
		c.httpClient = hc
	}
}

// WithLogger installs a minimal debug/warn logger. Optional; default is a
// no-op logger that drops all messages.
func WithLogger(l Logger) Option {
	return func(c *config) {
		c.logger = l
	}
}

// WithAutoLogin toggles automatic Login on the first authenticated call.
// Default: true. When false, callers must invoke client.User.Login(ctx)
// explicitly or set the session out-of-band before issuing API calls.
func WithAutoLogin(enabled bool) Option {
	return func(c *config) {
		c.autoLogin = enabled
	}
}

// WithSessionStore installs a SessionStore for cross-process session
// persistence. The default is NoopSessionStore{} (no persistence). With a
// store installed:
//
//   - NewClient calls store.Load(ctx) and seeds the in-memory session from it
//     (unless WithAutoLoadSession(false) is also set).
//   - client.User.Login() and the transparent 103 re-login both Save() the
//     new session.
//   - client.User.Logout() Clear()s the store.
//
// Common implementations:
//
//   - NewFileSessionStore(path) for a JSON file persisted to disk.
//   - A caller-defined struct for keychain, Redis, vault, etc.
//
// Store failures are non-fatal: errors are logged via the configured Logger
// and the SDK continues operating from the in-memory session.
func WithSessionStore(s SessionStore) Option {
	return func(c *config) {
		if s == nil {
			s = NoopSessionStore{}
		}
		c.sessionStore = s
	}
}

// WithAutoLoadSession controls whether NewClient calls SessionStore.Load on
// construction. Default: true. Disable when you want to install a store for
// Save() side-effects only (e.g. recording-only audit stores) without
// hydrating from a possibly-stale persisted session.
func WithAutoLoadSession(enabled bool) Option {
	return func(c *config) {
		c.autoLoadSession = enabled
	}
}

// Logger is a minimal, optional debug/warn sink. Implementations should be
// safe for concurrent use; the SDK does not synchronize calls.
type Logger interface {
	Debug(msg string, kv ...any)
	Warn(msg string, kv ...any)
}

// noopLogger is the default Logger when WithLogger is not used.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}
