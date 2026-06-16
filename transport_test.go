package yukiyama

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeRoundTripper records the requests it sees and returns canned responses
// in order. When responses is exhausted it returns the final response again.
type fakeRoundTripper struct {
	mu        atomic.Int32
	requests  []*http.Request
	responses []*http.Response
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Capture a snapshot of req.URL so later mutation by callers (e.g.
	// req.URL.Query() reassignment) doesn't change what we observe.
	cloned := *req
	cloned.URL = cloneURL(req.URL)
	cloned.Header = req.Header.Clone()
	f.requests = append(f.requests, &cloned)

	idx := int(f.mu.Add(1)) - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	resp := f.responses[idx]
	// Reset body so multiple replays of the canned response still readable.
	if resp.Body != nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		// Refresh the canned response with a fresh body for any future reads.
		f.responses[idx] = &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	return resp, nil
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	c := *u
	return &c
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestInjectAuth_AddsQueryParams verifies user_id, token, and version land on
// the outgoing URL when a session is present and the caller did NOT preempt
// any of them — i.e. the common case where the gen builder leaves
// user_id/token empty for the transport to fill.
func TestInjectAuth_AddsQueryParams(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "test-ua", autoLogin: false}, session: &Session{}}
	c.session.Set(12345, "deadbeefcafef00d")

	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":true}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/user/get?id=12345&is_home=0", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	got := tr.base.(*fakeRoundTripper).requests[0]
	q := got.URL.Query()
	if q.Get("user_id") != "12345" {
		t.Errorf("user_id: got %q, want 12345 (session must fill when empty)", q.Get("user_id"))
	}
	if q.Get("token") != "deadbeefcafef00d" {
		t.Errorf("token: got %q (session must fill when empty)", q.Get("token"))
	}
	if q.Get("version") != APIVersionName {
		t.Errorf("version: got %q, want %s", q.Get("version"), APIVersionName)
	}
	// Pre-existing params preserved.
	if q.Get("id") != "12345" {
		t.Errorf("id: got %q", q.Get("id"))
	}
	if q.Get("is_home") != "0" {
		t.Errorf("is_home: got %q", q.Get("is_home"))
	}
	if got.Header.Get("User-Agent") != "test-ua" {
		t.Errorf("UA: got %q", got.Header.Get("User-Agent"))
	}
}

// TestInjectAuth_PreservesCallerUserID verifies that when a caller (typically
// a facade like CheckUserNameAvailable) has already populated `user_id=` on
// the outbound URL, the transport does NOT clobber it with the session id.
// All three members of the auth triple follow the same "caller wins" rule;
// see CheckUserNameAvailable in facade.go for the motivating use case (wire
// param `user_id` carrying a username string).
func TestInjectAuth_PreservesCallerUserID(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "test-ua", autoLogin: false}, session: &Session{}}
	c.session.Set(12345, "deadbeefcafef00d")

	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":true}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/user/check_user_id_available?user_id=desired_handle", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got := tr.base.(*fakeRoundTripper).requests[0]
	q := got.URL.Query()
	if q.Get("user_id") != "desired_handle" {
		t.Errorf("user_id: got %q, want \"desired_handle\" (caller-set string must not be overwritten by session id %d)",
			q.Get("user_id"), 12345)
	}
	// token must still land from the session.
	if q.Get("token") != "deadbeefcafef00d" {
		t.Errorf("token: got %q, want session token", q.Get("token"))
	}
}

// TestInjectAuth_PreservesCallerToken verifies the same caller-wins policy
// for the `token` param. Tests guard against regressions that would
// special-case one parameter and break the uniform user_id/token/version
// treatment.
func TestInjectAuth_PreservesCallerToken(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "test-ua", autoLogin: false}, session: &Session{}}
	c.session.Set(12345, "deadbeefcafef00d")

	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":true}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/user/get?token=caller_supplied", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got := tr.base.(*fakeRoundTripper).requests[0]
	q := got.URL.Query()
	if q.Get("token") != "caller_supplied" {
		t.Errorf("token: got %q, want \"caller_supplied\" (caller-set value must not be overwritten by session)", q.Get("token"))
	}
	if q.Get("user_id") != "12345" {
		t.Errorf("user_id: got %q, want session id (session must still fill when empty)", q.Get("user_id"))
	}
}

// TestInjectAuth_PreservesCallerVersion verifies that when a caller (typically
// a facade method) has already populated `version=<content-schema-selector>`
// on the request URL, the transport does NOT clobber it with APIVersionName.
// Endpoints that need this: getHomeData ("5"), getUnreadCount ("2"),
// getUserProfile ("3"), listDistributionNotifications ("2").
func TestInjectAuth_PreservesCallerVersion(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "test-ua", autoLogin: false}, session: &Session{}}
	c.session.Set(12345, "deadbeefcafef00d")

	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":true}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/common/get_home_data?version=5", nil)
	_, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got := tr.base.(*fakeRoundTripper).requests[0]
	q := got.URL.Query()
	if q.Get("version") != "5" {
		t.Errorf("version: got %q, want \"5\" (caller-provided content version must not be overwritten by APIVersionName=%s)",
			q.Get("version"), APIVersionName)
	}
	// Auth triple must still land.
	if q.Get("user_id") != "12345" {
		t.Errorf("user_id: got %q", q.Get("user_id"))
	}
	if q.Get("token") != "deadbeefcafef00d" {
		t.Errorf("token: got %q", q.Get("token"))
	}
}

// TestInjectAuth_SkipsLoginPaths verifies that authExempt paths are NOT
// injected with credentials.
func TestInjectAuth_SkipsLoginPaths(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "ua", autoLogin: false}, session: &Session{}}
	c.session.Set(99, "tok")

	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":"1","id":"1","token":"x","is_debugger":"0","profile":{},"error":""}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/user/login?mail=a&pw=b&platform=android", nil)
	_, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got := tr.base.(*fakeRoundTripper).requests[0]
	if got.URL.Query().Get("user_id") != "" {
		t.Errorf("login path should NOT have user_id; got %q", got.URL.Query().Get("user_id"))
	}
	if got.URL.Query().Get("token") != "" {
		t.Errorf("login path should NOT have token; got %q", got.URL.Query().Get("token"))
	}
}

// TestNoAutoLoginWithoutSession_ReturnsError ensures the autoLogin=false path
// surfaces an actionable error rather than silently sending blank credentials.
func TestNoAutoLoginWithoutSession_ReturnsError(t *testing.T) {
	c := &Client{cfg: &config{userAgent: "ua", autoLogin: false}, session: &Session{}}
	tr := &authTransport{
		base:       &fakeRoundTripper{responses: []*http.Response{newJSONResponse(200, `{"status":true}`)}},
		client:     c,
		maxRetries: 1,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://admin.yukiyama.biz/api/user/get", nil)
	_, err := tr.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error when no session and autoLogin disabled")
	}
}

// TestParseAPIErrorFromBody covers the success and failure envelopes plus
// the {status:"0"} string form.
func TestParseAPIErrorFromBody(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantErr  bool
		wantCode int32
		want103  bool
	}{
		{"bool false 103", `{"status":false,"error":"expired","error_code":103}`, true, 103, true},
		{"string 0", `{"status":"0","error":"bad"}`, true, 0, false},
		{"bool true success", `{"status":true,"id":1}`, false, 0, false},
		{"string 1 success", `{"status":"1","id":"1"}`, false, 0, false},
		{"no envelope (HTML)", `<html>oops</html>`, false, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAPIErrorFromBody(200, []byte(c.body))
			if (got != nil) != c.wantErr {
				t.Fatalf("wantErr=%v got=%v", c.wantErr, got)
			}
			if got != nil {
				if got.Code != c.wantCode {
					t.Errorf("code: got %d want %d", got.Code, c.wantCode)
				}
				if got.IsAuthExpired() != c.want103 {
					t.Errorf("IsAuthExpired: got %v want %v", got.IsAuthExpired(), c.want103)
				}
			}
		})
	}
}

// TestIsAuthExempt_StripsApiPrefix ensures apiBase variants that bake "/api"
// into the path still match the exempt set.
func TestIsAuthExempt_StripsApiPrefix(t *testing.T) {
	cases := map[string]bool{
		"/user/login":      true,
		"/api/user/login":  true,
		"/user/mail_auth":  true,
		"/api/user/regist": true,
		"/user/get":        false,
		"/api/user/get":    false,
	}
	for path, want := range cases {
		if got := isAuthExempt(path); got != want {
			t.Errorf("isAuthExempt(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestCodeOf_AndIsAuthExpired walks the error chain helpers.
func TestCodeOf_AndIsAuthExpired(t *testing.T) {
	apiErr := &APIError{Code: 103, Message: "expired", StatusCode: 200}
	if CodeOf(apiErr) != 103 {
		t.Errorf("CodeOf direct: got %d", CodeOf(apiErr))
	}
	if !IsAuthExpired(apiErr) {
		t.Error("IsAuthExpired direct: want true")
	}
	if CodeOf(nil) != 0 {
		t.Error("CodeOf(nil) should be 0")
	}
	if IsAuthExpired(nil) {
		t.Error("IsAuthExpired(nil) should be false")
	}
}
