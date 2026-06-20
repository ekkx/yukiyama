package yukiyama

import (
	"context"
	"errors"
	"strconv"

	gen "github.com/ekkx/yukiyama/gen"
)

// UserService groups all /user/* operations with session-aware,
// wire-quirk-correcting ergonomics. Obtain it via client.User.
//
// Methods follow the conventions documented at the top of skiarea.go: each
// call runs ensureSession at the top to lazily Login when WithAutoLogin(true)
// is set, then drives the underlying gen builder with the wire-quirk fixes
// (param-name reversals, content-schema version pins, target/caller swaps)
// described in each method's godoc.
//
// Session-lifecycle methods are co-located here as a coherent trio:
//
//   - Login    — authenticates with WithCredentials, populates session.
//   - Logout   — clears the in-memory session and the configured SessionStore.
//   - Withdraw — deletes the server-side account, then clears local state.
//
// Login and Withdraw call wire endpoints (/user/login, /user/withdrawal);
// Logout is local-only (yukiyama has no /user/logout endpoint, by single-
// session design) but lives here so all three lifecycle entry points sit
// next to each other. State inspection helpers (IsAuthenticated,
// CurrentUserID, CurrentToken, SetSession) remain on *Client itself.
type UserService struct {
	c *Client
}

// --- session lifecycle ------------------------------------------------------

// Login authenticates with the credentials configured via WithCredentials().
// The resulting (user_id, token) pair is cached in the session for all
// subsequent requests and persisted to the configured SessionStore.
// Underlying op: GET /user/login (operationId: loginWithEmail).
//
// Login is safe to call concurrently: an internal mutex serializes concurrent
// callers, and the double-check pattern means a second goroutine arriving
// after the first has succeeded becomes a no-op.
//
// Login is also invoked automatically by the underlying transport when:
//   - No session exists and WithAutoLogin(true) (the default) is set.
//   - A response is detected as {status:false, error_code:103} ("session
//     expired"); the transport clears the cache and re-logins exactly once.
//
// Unlike every other UserService method, Login is deliberately exempt from
// the ensureSession pre-check — it is the foundation that creates the
// session. Its lifecycle peers are Logout (local-only clear) and Withdraw
// (server-side account deletion).
func (s *UserService) Login(ctx context.Context) error {
	c := s.c
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
// asks the store to drop any persisted copy. Idempotent: it is safe to call
// without an active session.
//
// Note: this is a CLIENT-SIDE clear only; yukiyama follows a single-session
// model where logging in elsewhere invalidates the prior token. There is no
// /user/logout endpoint. Logout is co-located with Login and Withdraw as a
// lifecycle peer even though it never reaches the network.
//
// Store.Clear failures are logged via the configured Logger and otherwise
// swallowed — the in-memory clear always succeeds, which is the load-bearing
// invariant callers care about.
func (s *UserService) Logout() {
	s.c.session.Clear()
	if s.c.cfg != nil && s.c.cfg.sessionStore != nil {
		if err := s.c.cfg.sessionStore.Clear(context.Background()); err != nil {
			s.c.log().Warn("yukiyama: SessionStore.Clear failed", "err", err)
		}
	}
}

// Withdraw deletes the authenticated user's account. On success the in-memory
// session and the persistent SessionStore are both cleared, since the
// (user_id, token) pair is now permanently invalid on the server.
// Underlying op: GET /user/withdrawal (operationId: withdrawAccount).
//
// Withdraw is the wire-calling peer of Login in the lifecycle trio; Logout
// is the local-only peer.
func (s *UserService) Withdraw(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	_, _, err := s.c.api.UserAPI.WithdrawAccount(ctx).Execute()
	if err != nil {
		return err
	}
	// Server-side session is gone. Mirror that locally so subsequent calls
	// don't try to reuse a dead token. We deliberately call Logout (not
	// session.Clear) so the SessionStore is also wiped.
	s.Logout()
	return nil
}

// --- read: own / other profile ----------------------------------------------

// GetMyUserProfile fetches the authenticated user's full profile.
// Underlying op: GET /user/get (operationId: getUserProfile).
//
// Wire-version caveat: the `version` query parameter on this endpoint is a
// *content schema* selector. The official client uses "3"; we pin the same
// value so transport auto-injection of APIVersionName cannot silently swap
// the server-side response shape.
func (s *UserService) GetMyUserProfile(ctx context.Context) (*gen.UserGetResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.GetUserProfile(ctx).
		Id(s.c.CurrentUserID()).
		IsHome(0).
		Version("3").
		Execute()
	return res, err
}

// GetUserProfile fetches another user's profile by user_id.
// Underlying op: GET /user/get (operationId: getUserProfile).
//
// See GetMyUserProfile for the wire-version caveat — same endpoint, same pin.
func (s *UserService) GetUserProfile(ctx context.Context, userID int32) (*gen.UserGetResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.GetUserProfile(ctx).
		Id(userID).
		IsHome(0).
		Version("3").
		Execute()
	return res, err
}

// GetMyPageOptions packages the optional query parameters for GetMyPage.
type GetMyPageOptions struct {
	// Debugger flag (0/1).
	IsDebugger *int32
}

// GetMyPage fetches the "my page" landing payload for a target user
// (profile cards + recent activity rollup).
// Underlying op: GET /user/get_my_page (operationId: getMyPage).
//
// Wire-naming: `id` carries the target user_id; the caller's `user_id` is
// injected by the transport. Pass 0 for targetUserID to view your own page.
func (s *UserService) GetMyPage(ctx context.Context, targetUserID int32, opts GetMyPageOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.UserAPI.GetMyPage(ctx).Id(targetUserID)
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// CheckUserNameAvailable checks whether a desired public handle is available
// for registration. Underlying op: GET /user/check_user_id_available
// (operationId: checkUserIdAvailable).
//
// Wire-name caveat: this endpoint's `user_id` query parameter is a *username
// string*, NOT the caller's numeric user_id. We explicitly populate it via
// `.UserId(userName)` so that authTransport's "fill if missing" semantics
// for user_id do not silently overwrite the caller's handle with the
// session's numeric id. Without that preemption the server would see e.g.
// `user_id=12345` and return the existence check for the caller's own account.
func (s *UserService) CheckUserNameAvailable(ctx context.Context, userName string) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.CheckUserIdAvailable(ctx).
		UserId(userName).
		Execute()
	return res, err
}

// --- read: search ------------------------------------------------------------

// SearchUsersOptions packages the optional query parameters for SearchUsers.
type SearchUsersOptions struct {
	// JSON-encoded advanced search options.
	FindOption *string
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
}

// SearchUsers searches users by free-text keyword.
// Underlying op: GET /user/find (operationId: findUsers).
//
// user_id/token are injected by the transport.
func (s *UserService) SearchUsers(ctx context.Context, keyword string, opts SearchUsersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.FindUsers(ctx).Keyword(keyword)
	if opts.FindOption != nil {
		req = req.FindOption(*opts.FindOption)
	}
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListRecommendedUsersOptions is reserved for future optional params.
type ListRecommendedUsersOptions struct{}

// ListRecommendedUsers fetches the recommended-users carousel.
// Underlying op: GET /user/get_recommended_user (operationId: listRecommendedUsers).
func (s *UserService) ListRecommendedUsers(ctx context.Context, _ ListRecommendedUsersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListRecommendedUsers(ctx).Execute()
	return res, err
}

// --- read: follow graph ------------------------------------------------------

// ListUserFollowersOptions packages the optional query parameters for ListUserFollowers.
type ListUserFollowersOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Free-text filter on the follower list.
	Keyword *string
}

// ListUserFollowers fetches the follower list of a target user.
// Underlying op: GET /user/get_follower (operationId: listFollowers).
//
// REVERSED-NAMING CAVEAT: `id` carries the target user_id whose followers to
// list; the caller's `user_id` is injected by the transport. Pass 0 for
// targetUserID to view your own followers.
func (s *UserService) ListUserFollowers(ctx context.Context, targetUserID int32, opts ListUserFollowersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.UserAPI.ListFollowers(ctx).Id(targetUserID)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	if opts.Keyword != nil {
		req = req.Keyword(*opts.Keyword)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListUserFollowingOptions packages the optional query parameters for ListUserFollowing.
type ListUserFollowingOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Free-text filter on the following list.
	Keyword *string
}

// ListUserFollowing fetches the followees of a target user.
// Underlying op: GET /user/get_following (operationId: listFollowing).
//
// REVERSED-NAMING CAVEAT: `id` carries the target user_id whose followees
// to list; the caller's `user_id` is injected by the transport. Pass 0 for
// targetUserID to view your own followees.
func (s *UserService) ListUserFollowing(ctx context.Context, targetUserID int32, opts ListUserFollowingOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.UserAPI.ListFollowing(ctx).Id(targetUserID)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	if opts.Keyword != nil {
		req = req.Keyword(*opts.Keyword)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: schedules ---------------------------------------------------------

// ListUserSchedulesOptions packages the optional query parameters for
// ListUserSchedules. nil pointer fields are not transmitted.
//
// Wire-naming caveat (see ListUserSchedules godoc): the `Type` field maps to
// `type` on the wire — observed values include "list" (full list) and
// "month" (calendar view, requires From/To).
type ListUserSchedulesOptions struct {
	// View mode. Observed values: "list", "month".
	Type *string
	// Calendar window start date (YYYY-MM-DD). Empty when Type == "list".
	From *string
	// Calendar window end date (YYYY-MM-DD). Empty when Type == "list".
	To *string
}

// ListUserSchedules fetches the schedule entries (planned ride-outs) for a target
// user. Underlying op: GET /user/get_schedule (operationId: listSchedules).
//
// REVERSED-NAMING CAVEAT: unlike most endpoints in this API, this op's wire
// params `id` and `user_id` are *swapped*: `id` is the caller (authenticated
// user) and `user_id` is the target whose schedules are being queried. The
// facade hides that footgun — `targetUserID` is plumbed to `.UserId()` and
// `.Id()` is filled from CurrentUserID().
func (s *UserService) ListUserSchedules(ctx context.Context, targetUserID int32, opts ListUserSchedulesOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.ListSchedules(ctx).
		Id(s.c.CurrentUserID()).
		UserId(targetUserID)
	if opts.Type != nil {
		req = req.Type_(*opts.Type)
	}
	if opts.From != nil {
		req = req.From(*opts.From)
	}
	if opts.To != nil {
		req = req.To(*opts.To)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetScheduleDetail fetches a single schedule entry by its schedule_id.
// Underlying op: GET /user/get_schedule_detail (operationId: getScheduleDetail).
//
// Wire-naming: the caller user id is sent as `id` on this endpoint and is
// filled from CurrentUserID() internally.
func (s *UserService) GetScheduleDetail(ctx context.Context, scheduleID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.GetScheduleDetail(ctx).
		Id(s.c.CurrentUserID()).
		ScheduleId(scheduleID).
		Execute()
	return res, err
}

// DeleteSchedule deletes a schedule the caller owns.
// Underlying op: GET /user/delete_schedule (operationId: deleteSchedule).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) DeleteSchedule(ctx context.Context, scheduleID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.DeleteSchedule(ctx).
		Id(s.c.CurrentUserID()).
		ScheduleId(scheduleID).
		Execute()
	return statusOrError(res, err, "delete_schedule", "")
}

// --- read: visited skiareas --------------------------------------------------

// ListVisitedSkiareasOptions packages the optional query parameters for
// ListVisitedSkiareas.
type ListVisitedSkiareasOptions struct {
	// Debugger flag (0/1).
	IsDebugger *int32
}

// ListVisitedSkiareas fetches the lifetime list of ski areas the target user
// has visited at least once (no per-season aggregates — use
// ListVisitedSkiareasWithStats for that).
// Underlying op: GET /user/get_visit_skiarea (operationId: listVisitedSkiareas).
//
// Wire-param caveat: the wire request carries `target_user_id=` to scope the
// query. The current OpenAPI spec only declares is_debugger, so the gen
// builder lacks a `.TargetUserId()` setter; the facade plumbs the value via
// the transport's per-request extra-query escape hatch (withExtraQuery)
// until the spec gains the param in a future iteration.
// Pass 0 to request your own visits (uses CurrentUserID).
func (s *UserService) ListVisitedSkiareas(ctx context.Context, targetUserID int32, opts ListVisitedSkiareasOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	ctx = withExtraQuery(ctx, map[string]string{
		"target_user_id": strconv.FormatInt(int64(targetUserID), 10),
	})
	req := s.c.api.UserAPI.ListVisitedSkiareas(ctx)
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListVisitedSkiareasWithStatsOptions packages the optional query parameters
// for ListVisitedSkiareasWithStats.
//
// Wire-type notes:
//   - SeasonFrom/SeasonTo are YYYY-MM-DD strings.
//   - SeasonYear is a 4-digit calendar year (int32).
//   - Mode is a string; observed values: "visit", "checkin", "liftCount".
type ListVisitedSkiareasWithStatsOptions struct {
	// Season window start (YYYY-MM-DD).
	SeasonFrom *string
	// Season window end (YYYY-MM-DD).
	SeasonTo *string
	// Season year (4-digit calendar year).
	SeasonYear *int32
	// Aggregation mode. Observed values: "visit", "checkin", "liftCount".
	Mode *string
	// Debugger flag (0/1).
	IsDebugger *int32
}

// ListVisitedSkiareasWithStats fetches the visited-skiarea list scoped to a
// target user, with per-area visit/checkin/lift counts aggregated over the
// requested season window.
// Underlying op: GET /user/get_visit_skiarea_with_data
// (operationId: listVisitedSkiareasWithStats).
//
// Pass 0 for targetUserID to request your own aggregates (uses CurrentUserID).
// All season/mode filters are optional — omitting them yields the lifetime
// aggregate in the upstream's default mode.
func (s *UserService) ListVisitedSkiareasWithStats(ctx context.Context, targetUserID int32, opts ListVisitedSkiareasWithStatsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.UserAPI.ListVisitedSkiareasWithStats(ctx).
		TargetUserId(targetUserID)
	if opts.SeasonFrom != nil {
		req = req.SeasonFrom(*opts.SeasonFrom)
	}
	if opts.SeasonTo != nil {
		req = req.SeasonTo(*opts.SeasonTo)
	}
	if opts.SeasonYear != nil {
		req = req.SeasonYear(*opts.SeasonYear)
	}
	if opts.Mode != nil {
		req = req.Mode(*opts.Mode)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: rideouts / riding analysis ----------------------------------------

// ListRideoutsOptions packages the optional query parameters for ListRideouts.
type ListRideoutsOptions struct {
	// Season window start (YYYY-MM-DD).
	SeasonFrom *string
	// Season window end (YYYY-MM-DD).
	SeasonTo *string
	// Season year (string-encoded 4-digit calendar year).
	SeasonYear *string
}

// ListRideouts fetches the rideout (single-day ride session) list for the
// authenticated user, optionally scoped by season window.
// Underlying op: GET /user/get_rideouts (operationId: listRideouts).
//
// user_id/token are injected by the transport.
func (s *UserService) ListRideouts(ctx context.Context, opts ListRideoutsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.ListRideouts(ctx)
	if opts.SeasonFrom != nil {
		req = req.SeasonFrom(*opts.SeasonFrom)
	}
	if opts.SeasonTo != nil {
		req = req.SeasonTo(*opts.SeasonTo)
	}
	if opts.SeasonYear != nil {
		req = req.SeasonYear(*opts.SeasonYear)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetTotalRideoutOptions packages the optional query parameters for
// GetTotalRideout.
type GetTotalRideoutOptions struct {
	// Override user id used for the aggregate, intended for debugger sessions.
	DebugUserID *string
}

// GetTotalRideout fetches lifetime rideout aggregates for the authenticated
// user. Underlying op: GET /user/get_total_rideout (operationId: getTotalRideout).
func (s *UserService) GetTotalRideout(ctx context.Context, opts GetTotalRideoutOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.GetTotalRideout(ctx)
	if opts.DebugUserID != nil {
		req = req.DebugUserId(*opts.DebugUserID)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetRidingAnalysis fetches the riding-analysis payload for a target user.
// Underlying op: GET /user/get_riding_analysis (operationId: getRidingAnalysis).
//
// Wire-naming: `id` carries the target user_id; the caller's `user_id` is
// injected by the transport. Pass 0 to analyse your own riding.
func (s *UserService) GetRidingAnalysis(ctx context.Context, targetUserID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	res, _, err := s.c.api.UserAPI.GetRidingAnalysis(ctx).
		Id(targetUserID).
		Execute()
	return res, err
}

// GetRidingGraphOptions packages the optional query parameters for
// GetRidingGraph.
//
// Wire-type notes: all filters are strings on the wire; the caller chooses
// the time window and aggregation slice via SelectTerm / SelectTarget /
// PreTarget. DebugUserID overrides the user scope for debugger sessions.
type GetRidingGraphOptions struct {
	// Time window selector (e.g. "season", "month").
	SelectTerm *string
	// Aggregation slice selector (e.g. "distance", "altitude").
	SelectTarget *string
	// Comparison-slice selector (paired with SelectTarget).
	PreTarget *string
	// Override user id for the graph, intended for debugger sessions.
	DebugUserID *string
}

// GetRidingGraph fetches the riding-graph payload (time-series aggregate).
// Underlying op: GET /user/get_riding_graph (operationId: getRidingGraph).
func (s *UserService) GetRidingGraph(ctx context.Context, opts GetRidingGraphOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.GetRidingGraph(ctx)
	if opts.SelectTerm != nil {
		req = req.SelectTerm(*opts.SelectTerm)
	}
	if opts.SelectTarget != nil {
		req = req.SelectTarget(*opts.SelectTarget)
	}
	if opts.PreTarget != nil {
		req = req.PreTarget(*opts.PreTarget)
	}
	if opts.DebugUserID != nil {
		req = req.DebugUserId(*opts.DebugUserID)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: checkin seasons ---------------------------------------------------

// ListCheckinSeasonsOptions packages the optional query parameters for
// ListCheckinSeasons.
type ListCheckinSeasonsOptions struct {
	// Debugger flag (0/1).
	IsDebugger *int32
}

// ListCheckinSeasons fetches the season buckets (per-season checkin counts)
// for a target user. Underlying op: GET /user/get_checkin_seasons
// (operationId: listCheckinSeasons).
//
// Wire-naming: `id` carries the target user_id; the caller's `user_id` is
// injected by the transport. Pass 0 for your own seasons.
func (s *UserService) ListCheckinSeasons(ctx context.Context, targetUserID int32, opts ListCheckinSeasonsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.UserAPI.ListCheckinSeasons(ctx).Id(targetUserID)
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: glided together ---------------------------------------------------

// ListGlidedTogetherUsersOptions packages the optional query parameters for
// ListGlidedTogetherUsers.
type ListGlidedTogetherUsersOptions struct {
	// Season window start (YYYY-MM-DD).
	SeasonFrom *string
	// Season window end (YYYY-MM-DD).
	SeasonTo *string
	// Season year (string-encoded 4-digit calendar year).
	SeasonYear *string
}

// ListGlidedTogetherUsers fetches the list of users the caller has glided together
// with, optionally scoped by season window.
// Underlying op: GET /user/get_glided_together (operationId: listGlidedTogether).
func (s *UserService) ListGlidedTogetherUsers(ctx context.Context, opts ListGlidedTogetherUsersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.ListGlidedTogether(ctx)
	if opts.SeasonFrom != nil {
		req = req.SeasonFrom(*opts.SeasonFrom)
	}
	if opts.SeasonTo != nil {
		req = req.SeasonTo(*opts.SeasonTo)
	}
	if opts.SeasonYear != nil {
		req = req.SeasonYear(*opts.SeasonYear)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: favorites / coupons / my groups -----------------------------------

// ListFavoriteSkiareasOptions packages the optional query parameters for
// ListFavoriteSkiareas.
type ListFavoriteSkiareasOptions struct {
	// Restrict the listing to a single skiarea_id (used for existence checks).
	SkiareaID *int32
}

// ListFavoriteSkiareas fetches the caller's favorite ski areas.
// Underlying op: GET /user/get_favorite (operationId: listFavoriteSkiareas).
func (s *UserService) ListFavoriteSkiareas(ctx context.Context, opts ListFavoriteSkiareasOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.ListFavoriteSkiareas(ctx)
	if opts.SkiareaID != nil {
		req = req.SkiareaId(*opts.SkiareaID)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListMyCouponsOptions is reserved for future optional params.
type ListMyCouponsOptions struct{}

// ListMyCoupons fetches the caller's coupon wallet.
// Underlying op: GET /user/get_my_coupon (operationId: listMyCoupons).
func (s *UserService) ListMyCoupons(ctx context.Context, _ ListMyCouponsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListMyCoupons(ctx).Execute()
	return res, err
}

// ListMyUserGroupsOptions is reserved for future optional params.
type ListMyUserGroupsOptions struct{}

// ListMyUserGroups fetches the caller's joined group list.
// Underlying op: GET /user/get_my_group (operationId: listMyGroups).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) ListMyUserGroups(ctx context.Context, _ ListMyUserGroupsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListMyGroups(ctx).
		Id(s.c.CurrentUserID()).
		Execute()
	return res, err
}

// AddGroupFromMyGroup adds the caller's existing group membership to a
// skiarea as a quick join shortcut.
// Underlying op: GET /user/add_group_from_my_group
// (operationId: addGroupFromMyGroup).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) AddGroupFromMyGroup(ctx context.Context, skiareaID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.AddGroupFromMyGroup(ctx).
		Id(s.c.CurrentUserID()).
		SkiareaId(skiareaID).
		Execute()
	return statusOrError(res, err, "add_group_from_my_group", "")
}

// DeleteMyGroup removes a group from the caller's joined group list.
// Underlying op: GET /user/delete_my_group (operationId: deleteMyGroup).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
// The group's id is encoded on the wire as the `id` query parameter, but
// here the caller user id occupies `id` and the operation deletes the
// group bound to that user by the upstream session — the gen builder
// exposes no group_id setter.
func (s *UserService) DeleteMyGroup(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.DeleteMyGroup(ctx).
		Id(s.c.CurrentUserID()).
		Execute()
	return statusOrError(res, err, "delete_my_group", "")
}

// --- read: badges ------------------------------------------------------------

// ListBadgesOptions is reserved for future optional params.
type ListBadgesOptions struct{}

// ListBadges fetches the badge wallet for the authenticated user.
// Underlying op: GET /user/get_badges (operationId: listBadges).
func (s *UserService) ListBadges(ctx context.Context, _ ListBadgesOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListBadges(ctx).Execute()
	return res, err
}

// ListNewBadgesOptions is reserved for future optional params.
type ListNewBadgesOptions struct{}

// ListNewBadges fetches the unseen-badge list (badges newly awarded since
// the last viewBadge ack).
// Underlying op: GET /user/get_new_badges (operationId: listNewBadges).
func (s *UserService) ListNewBadges(ctx context.Context, _ ListNewBadgesOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListNewBadges(ctx).Execute()
	return res, err
}

// ViewBadge acknowledges that the caller has seen a badge (clears its
// new-badge flag).
// Underlying op: GET /user/view_badge (operationId: viewBadge).
func (s *UserService) ViewBadge(ctx context.Context, badgeLogID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.ViewBadge(ctx).
		BadgeLogId(badgeLogID).
		Execute()
	return statusOrError(res, err, "view_badge", "")
}

// ViewEventBadge acknowledges that the caller has seen an event badge.
// Underlying op: GET /user/view_event_badge (operationId: viewEventBadge).
func (s *UserService) ViewEventBadge(ctx context.Context, badgeID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.ViewEventBadge(ctx).
		BadgeId(badgeID).
		Execute()
	return statusOrError(res, err, "view_event_badge", "")
}

// UpdateBadgeSettingOptions packages the optional fields for UpdateBadgeSetting.
// Type / Rank / Index are float32 on the wire (selectors), IsDelete is the
// soft-delete flag (0/1), EventBadgeID is set only for event-badge slots.
type UpdateBadgeSettingOptions struct {
	// Badge slot type selector.
	Type *float32
	// Badge rank selector.
	Rank *float32
	// Slot index.
	Index *float32
	// Soft-delete flag (0/1).
	IsDelete *int32
	// Event badge id (event-badge slots only).
	EventBadgeID *int32
}

// UpdateBadgeSetting updates a single badge-display slot.
// Underlying op: GET /user/update_badge_setting (operationId: updateBadgeSetting).
func (s *UserService) UpdateBadgeSetting(ctx context.Context, opts UpdateBadgeSettingOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.UpdateBadgeSetting(ctx)
	if opts.Type != nil {
		req = req.Type_(*opts.Type)
	}
	if opts.Rank != nil {
		req = req.Rank(*opts.Rank)
	}
	if opts.Index != nil {
		req = req.Index(*opts.Index)
	}
	if opts.IsDelete != nil {
		req = req.IsDelete(*opts.IsDelete)
	}
	if opts.EventBadgeID != nil {
		req = req.EventBadgeId(*opts.EventBadgeID)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "update_badge_setting", "")
}

// UpdateAllBadgeSettings replaces the full badge-display configuration in one
// call. settingAll is a JSON-encoded array describing every slot.
// Underlying op: GET /user/update_all_badge_settings
// (operationId: updateAllBadgeSettings).
func (s *UserService) UpdateAllBadgeSettings(ctx context.Context, settingAll string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateAllBadgeSettings(ctx).
		SettingAll(settingAll).
		Execute()
	return statusOrError(res, err, "update_all_badge_settings", "")
}

// --- read: stances -----------------------------------------------------------

// ListAllStancesOptions is reserved for future optional params.
type ListAllStancesOptions struct{}

// ListAllStances fetches every stance entry for a target user.
// Underlying op: GET /user/get_all_stances (operationId: listAllStances).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
// `user_id` is injected by the transport.
func (s *UserService) ListAllStances(ctx context.Context, _ ListAllStancesOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListAllStances(ctx).
		Id(s.c.CurrentUserID()).
		Execute()
	return res, err
}

// ListStancesNewOptions packages the optional query parameters for
// ListStancesNew.
type ListStancesNewOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
}

// ListStancesNew fetches the paginated stance feed (newer schema variant).
// Underlying op: GET /user/get_stances_new (operationId: listStancesNew).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) ListStancesNew(ctx context.Context, opts ListStancesNewOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.UserAPI.ListStancesNew(ctx).Id(s.c.CurrentUserID())
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetStanceDetail fetches a single stance entry by stance_id.
// Underlying op: GET /user/get_stance_detail (operationId: getStanceDetail).
func (s *UserService) GetStanceDetail(ctx context.Context, stanceID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.GetStanceDetail(ctx).
		StanceId(stanceID).
		Execute()
	return res, err
}

// AddStance creates a new stance entry. The current spec does not model a
// query body for this op; the upstream client posts stance fields via a
// multipart payload that this SDK does not yet surface. The facade is
// kept as a thin pass-through so callers can drive the raw transport
// when the spec gains the body schema.
// Underlying op: GET /user/add_stance (operationId: addStance).
func (s *UserService) AddStance(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.AddStance(ctx).Execute()
	return statusOrError(res, err, "add_stance", "")
}

// DeleteStance removes a stance entry by stance_id.
// Underlying op: GET /user/delete_stance (operationId: deleteStance).
func (s *UserService) DeleteStance(ctx context.Context, stanceID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.DeleteStance(ctx).
		StanceId(stanceID).
		Execute()
	return statusOrError(res, err, "delete_stance", "")
}

// UpdateStanceMemo edits the free-form memo on a stance entry. The current
// spec does not model the memo/stance_id body for this op (the upstream
// client posts them as a body payload that this SDK does not yet surface);
// the facade is a thin pass-through pending a spec update.
// Underlying op: GET /user/update_stance_memo (operationId: updateStanceMemo).
func (s *UserService) UpdateStanceMemo(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateStanceMemo(ctx).Execute()
	return statusOrError(res, err, "update_stance_memo", "")
}

// UpdateStanceSetting toggles the privacy flag (stance_visible) on the
// caller's stance feed.
// Underlying op: GET /user/update_stance_setting (operationId: updateStanceSetting).
func (s *UserService) UpdateStanceSetting(ctx context.Context, stanceVisible int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateStanceSetting(ctx).
		StanceVisible(stanceVisible).
		Execute()
	return statusOrError(res, err, "update_stance_setting", "")
}

// UpdateFavoriteStance promotes a stance entry to a slot in the favorite-stance
// carousel at the given (0-based) index.
// Underlying op: GET /user/update_favorite_stance
// (operationId: updateFavoriteStance).
func (s *UserService) UpdateFavoriteStance(ctx context.Context, stanceID int32, stanceIndex float32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateFavoriteStance(ctx).
		StanceId(stanceID).
		StanceIndex(stanceIndex).
		Execute()
	return statusOrError(res, err, "update_favorite_stance", "")
}

// UpdateFavoriteStanceCustomOptions packages the optional fields for
// UpdateFavoriteStanceCustom.
type UpdateFavoriteStanceCustomOptions struct {
	// Custom L (left) angle, in degrees.
	CustomL *float32
	// Custom R (right) angle, in degrees.
	CustomR *float32
	// Custom W (width) value, in centimeters.
	CustomW *float32
}

// UpdateFavoriteStanceCustom updates the "custom" slot in the favorite-stance
// carousel. Pass isCustom=1 to mark the slot active and populate any of
// CustomL / CustomR / CustomW in opts.
// Underlying op: GET /user/update_favorite_stance_custom
// (operationId: updateFavoriteStanceCustom).
func (s *UserService) UpdateFavoriteStanceCustom(ctx context.Context, isCustom int32, opts UpdateFavoriteStanceCustomOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.UpdateFavoriteStanceCustom(ctx).
		FavoriteStanceIsCustom(isCustom)
	if opts.CustomL != nil {
		req = req.FavoriteStanceCustomL(*opts.CustomL)
	}
	if opts.CustomR != nil {
		req = req.FavoriteStanceCustomR(*opts.CustomR)
	}
	if opts.CustomW != nil {
		req = req.FavoriteStanceCustomW(*opts.CustomW)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "update_favorite_stance_custom", "")
}

// ClearFavoriteStance clears every slot in the favorite-stance carousel.
// Underlying op: GET /user/clear_favorite_stance
// (operationId: clearFavoriteStance).
func (s *UserService) ClearFavoriteStance(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.ClearFavoriteStance(ctx).Execute()
	return statusOrError(res, err, "clear_favorite_stance", "")
}

// --- read: blocked users / geofencing ---------------------------------------

// ListBlockedUsersOptions is reserved for future optional params.
type ListBlockedUsersOptions struct{}

// ListBlockedUsers fetches the caller's blocked-user list.
// Underlying op: GET /user/get_blocked_users (operationId: listBlockedUsers).
func (s *UserService) ListBlockedUsers(ctx context.Context, _ ListBlockedUsersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListBlockedUsers(ctx).Execute()
	return res, err
}

// BlockUser adds the target user to the caller's block list.
// Underlying op: GET /user/add_user_block (operationId: addUserBlock).
func (s *UserService) BlockUser(ctx context.Context, targetUserID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.AddUserBlock(ctx).
		TargetUserId(targetUserID).
		Execute()
	return statusOrError(res, err, "add_user_block", "")
}

// UnblockUser removes the target user from the caller's block list.
// Underlying op: GET /user/remove_user_block (operationId: removeUserBlock).
func (s *UserService) UnblockUser(ctx context.Context, targetUserID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.RemoveUserBlock(ctx).
		TargetUserId(targetUserID).
		Execute()
	return statusOrError(res, err, "remove_user_block", "")
}

// ListGeofencingTargetsOptions is reserved for future optional params.
type ListGeofencingTargetsOptions struct{}

// ListGeofencingTargets fetches the geofencing targets configured for the
// caller (regions that trigger background checkin alerts).
// Underlying op: GET /user/get_geofencing_targets
// (operationId: listGeofencingTargets).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) ListGeofencingTargets(ctx context.Context, _ ListGeofencingTargetsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.UserAPI.ListGeofencingTargets(ctx).
		Id(s.c.CurrentUserID()).
		Execute()
	return res, err
}

// --- mutations: follow (cmd-split) ------------------------------------------

// FollowUser adds the target user to the caller's follow list.
// Underlying op: GET /user/update_follow (operationId: updateFollow)
// with cmd="add".
func (s *UserService) FollowUser(ctx context.Context, targetUserID int32) error {
	return s.updateFollow(ctx, targetUserID, "add")
}

// UnfollowUser removes the target user from the caller's follow list.
// Underlying op: GET /user/update_follow with cmd="delete".
func (s *UserService) UnfollowUser(ctx context.Context, targetUserID int32) error {
	return s.updateFollow(ctx, targetUserID, "delete")
}

func (s *UserService) updateFollow(ctx context.Context, targetUserID int32, cmd string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateFollow(ctx).
		FollowUserId(targetUserID).
		Cmd(cmd).
		Execute()
	return statusOrError(res, err, "update_follow", cmd)
}

// --- mutations: favorite skiarea (cmd-split) --------------------------------

// AddFavoriteSkiarea adds a skiarea to the caller's favorites.
// Underlying op: GET /user/update_favorite (operationId: updateFavoriteSkiarea)
// with cmd="add".
func (s *UserService) AddFavoriteSkiarea(ctx context.Context, skiareaID int32) error {
	return s.updateFavoriteSkiarea(ctx, skiareaID, "add")
}

// RemoveFavoriteSkiarea removes a skiarea from the caller's favorites.
// Underlying op: GET /user/update_favorite with cmd="delete".
func (s *UserService) RemoveFavoriteSkiarea(ctx context.Context, skiareaID int32) error {
	return s.updateFavoriteSkiarea(ctx, skiareaID, "delete")
}

func (s *UserService) updateFavoriteSkiarea(ctx context.Context, skiareaID int32, cmd string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateFavoriteSkiarea(ctx).
		SkiareaId(skiareaID).
		Cmd(cmd).
		Execute()
	return statusOrError(res, err, "update_favorite", cmd)
}

// UpdateFavoriteSkiareasSort persists a custom sort order for the caller's favorite
// ski areas. sortList is the JSON-encoded ordered array of skiarea_ids.
// Underlying op: GET /user/update_favorite_sort
// (operationId: updateFavoriteSort).
func (s *UserService) UpdateFavoriteSkiareasSort(ctx context.Context, sortList string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateFavoriteSort(ctx).
		SortList(sortList).
		Execute()
	return statusOrError(res, err, "update_favorite_sort", "")
}

// --- mutations: schedule join (cmd-split) -----------------------------------

// JoinScheduleOptions packages the optional flags for JoinSchedule.
type JoinScheduleOptions struct {
	// 0/1 — toggles the per-schedule checkin-alert push opt-in.
	IsPushCheckinAlert *int32
}

// JoinSchedule joins the caller to a planned schedule (ride-out).
// Underlying op: GET /user/update_schedule_join
// (operationId: updateScheduleJoin) with cmd="join".
//
// Wire-naming: the caller user id is sent as `id` on this endpoint; filled
// from CurrentUserID().
func (s *UserService) JoinSchedule(ctx context.Context, scheduleID int32, opts JoinScheduleOptions) error {
	return s.updateScheduleJoin(ctx, scheduleID, "join", opts.IsPushCheckinAlert)
}

// LeaveSchedule removes the caller from a planned schedule.
// Underlying op: GET /user/update_schedule_join with cmd="delete".
func (s *UserService) LeaveSchedule(ctx context.Context, scheduleID int32) error {
	return s.updateScheduleJoin(ctx, scheduleID, "delete", nil)
}

func (s *UserService) updateScheduleJoin(ctx context.Context, scheduleID int32, cmd string, isPushCheckinAlert *int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.UpdateScheduleJoin(ctx).
		Id(s.c.CurrentUserID()).
		ScheduleId(scheduleID).
		Cmd(cmd)
	if isPushCheckinAlert != nil {
		req = req.IsPushCheckinAlert(*isPushCheckinAlert)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "update_schedule_join", cmd)
}

// UpdateScheduleCheckinAlertPush toggles the caller's per-schedule
// checkin-alert push setting independently of join state.
// Underlying op: GET /user/update_schedule_checkin_alert_push
// (operationId: updateScheduleCheckinAlertPush).
//
// Wire-naming: the caller user id is sent as `id`; filled from CurrentUserID().
func (s *UserService) UpdateScheduleCheckinAlertPush(ctx context.Context, scheduleID, isPushCheckinAlert int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UpdateScheduleCheckinAlertPush(ctx).
		Id(s.c.CurrentUserID()).
		ScheduleId(scheduleID).
		IsPushCheckinAlert(isPushCheckinAlert).
		Execute()
	return statusOrError(res, err, "update_schedule_checkin_alert_push", "")
}

// --- mutations: profile / user edit -----------------------------------------

// EditUserOptions packages the optional fields for EditUser.
//
// EditUser covers the "core profile" half of the editable fields (nickname,
// appeal text, social URLs). For the gear / personal-attribute half use
// EditProfile; for the consolidated post-migration shape use EditUserNew.
type EditUserOptions struct {
	// Display name.
	Nickname *string
	// Free-form bio text shown on the profile page.
	Appeal *string
	// Public Instagram URL.
	InstagramURL *string
	// Public Facebook URL.
	FacebookURL *string
	// Public X (Twitter) URL.
	TwitterURL *string
	// Public YouTube URL.
	YoutubeURL *string
	// Public TikTok URL.
	TiktokURL *string
}

// EditUser updates the caller's profile core fields.
// Underlying op: GET /user/edit_user (operationId: editUser).
func (s *UserService) EditUser(ctx context.Context, opts EditUserOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.EditUser(ctx)
	if opts.Nickname != nil {
		req = req.Nickname(*opts.Nickname)
	}
	if opts.Appeal != nil {
		req = req.Appeal(*opts.Appeal)
	}
	if opts.InstagramURL != nil {
		req = req.InstagramUrl(*opts.InstagramURL)
	}
	if opts.FacebookURL != nil {
		req = req.FacebookUrl(*opts.FacebookURL)
	}
	if opts.TwitterURL != nil {
		req = req.TwitterUrl(*opts.TwitterURL)
	}
	if opts.YoutubeURL != nil {
		req = req.YoutubeUrl(*opts.YoutubeURL)
	}
	if opts.TiktokURL != nil {
		req = req.TiktokUrl(*opts.TiktokURL)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "edit_user", "")
}

// EditProfileOptions packages the optional fields for EditProfile.
//
// EditProfile covers the "personal attributes" half (gear, birthday, style,
// home skiarea, per-field visibility flags). For the social-URL half use
// EditUser; for the consolidated post-migration shape use EditUserNew.
type EditProfileOptions struct {
	// Free-form text describing the caller's primary gear.
	MainGear *string
	// Birthday (YYYY-MM-DD).
	Birthday *string
	// Gender selector (wire type: float32).
	Gender *float32
	// Birthplace (free-form text).
	Birthplace *string
	// Style selector (e.g. "freestyle", "carving").
	Style *string
	// Self-reported level.
	Level *string
	// Self-reported years of experience.
	Experience *string
	// Home skiarea id.
	HomeSkiareaID *int32
	// Visibility flag for birth month/day (0/1).
	BirthmonthdayVisible *int32
	// Visibility flag for birth year (0/1).
	BirthyearVisible *int32
	// Visibility flag for birthplace (0/1).
	BirthplaceVisible *int32
	// Visibility flag for the supporter label on the profile card (0/1).
	IsShowSupporterLabel *int32
}

// EditProfile updates the caller's personal-attribute fields.
// Underlying op: GET /user/edit_profile (operationId: editProfile).
func (s *UserService) EditProfile(ctx context.Context, opts EditProfileOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.EditProfile(ctx)
	if opts.MainGear != nil {
		req = req.MainGear(*opts.MainGear)
	}
	if opts.Birthday != nil {
		req = req.Birthday(*opts.Birthday)
	}
	if opts.Gender != nil {
		req = req.Gender(*opts.Gender)
	}
	if opts.Birthplace != nil {
		req = req.Birthplace(*opts.Birthplace)
	}
	if opts.Style != nil {
		req = req.Style(*opts.Style)
	}
	if opts.Level != nil {
		req = req.Level(*opts.Level)
	}
	if opts.Experience != nil {
		req = req.Experience(*opts.Experience)
	}
	if opts.HomeSkiareaID != nil {
		req = req.HomeskiareaId(*opts.HomeSkiareaID)
	}
	if opts.BirthmonthdayVisible != nil {
		req = req.BirthmonthdayVisible(*opts.BirthmonthdayVisible)
	}
	if opts.BirthyearVisible != nil {
		req = req.BirthyearVisible(*opts.BirthyearVisible)
	}
	if opts.BirthplaceVisible != nil {
		req = req.BirthplaceVisible(*opts.BirthplaceVisible)
	}
	if opts.IsShowSupporterLabel != nil {
		req = req.IsShowSupporterLabel(*opts.IsShowSupporterLabel)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "edit_profile", "")
}

// EditUserNewOptions packages the optional fields for EditUserNew.
//
// EditUserNew is the post-migration consolidated edit endpoint and accepts
// the union of EditUser and EditProfile fields. Prefer EditUserNew on new
// integrations; the split EditUser/EditProfile pair is retained for parity
// with older upstream clients.
type EditUserNewOptions struct {
	// Display name.
	Nickname *string
	// Free-form bio text.
	Appeal *string
	// Public Instagram URL.
	InstagramURL *string
	// Public Facebook URL.
	FacebookURL *string
	// Public X (Twitter) URL.
	TwitterURL *string
	// Public YouTube URL.
	YoutubeURL *string
	// Public TikTok URL.
	TiktokURL *string
	// Free-form text describing the caller's primary gear.
	MainGear *string
	// Birthday (YYYY-MM-DD).
	Birthday *string
	// Gender selector (wire type: float32).
	Gender *float32
	// Birthplace (free-form text).
	Birthplace *string
	// Style selector.
	Style *string
	// Self-reported level.
	Level *string
	// Self-reported years of experience.
	Experience *string
	// Home skiarea id.
	HomeSkiareaID *int32
	// Visibility flag for birth month/day (0/1).
	BirthmonthdayVisible *int32
	// Visibility flag for birth year (0/1).
	BirthyearVisible *int32
	// Visibility flag for birthplace (0/1).
	BirthplaceVisible *int32
	// Visibility flag for the supporter label on the profile card (0/1).
	IsShowSupporterLabel *int32
}

// EditUserNew updates the caller's full profile via the consolidated
// post-migration endpoint. Prefer this over EditUser / EditProfile on new
// integrations.
// Underlying op: GET /user/edit_user_new (operationId: editUserNew).
func (s *UserService) EditUserNew(ctx context.Context, opts EditUserNewOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.EditUserNew(ctx)
	if opts.Nickname != nil {
		req = req.Nickname(*opts.Nickname)
	}
	if opts.Appeal != nil {
		req = req.Appeal(*opts.Appeal)
	}
	if opts.InstagramURL != nil {
		req = req.InstagramUrl(*opts.InstagramURL)
	}
	if opts.FacebookURL != nil {
		req = req.FacebookUrl(*opts.FacebookURL)
	}
	if opts.TwitterURL != nil {
		req = req.TwitterUrl(*opts.TwitterURL)
	}
	if opts.YoutubeURL != nil {
		req = req.YoutubeUrl(*opts.YoutubeURL)
	}
	if opts.TiktokURL != nil {
		req = req.TiktokUrl(*opts.TiktokURL)
	}
	if opts.MainGear != nil {
		req = req.MainGear(*opts.MainGear)
	}
	if opts.Birthday != nil {
		req = req.Birthday(*opts.Birthday)
	}
	if opts.Gender != nil {
		req = req.Gender(*opts.Gender)
	}
	if opts.Birthplace != nil {
		req = req.Birthplace(*opts.Birthplace)
	}
	if opts.Style != nil {
		req = req.Style(*opts.Style)
	}
	if opts.Level != nil {
		req = req.Level(*opts.Level)
	}
	if opts.Experience != nil {
		req = req.Experience(*opts.Experience)
	}
	if opts.HomeSkiareaID != nil {
		req = req.HomeskiareaId(*opts.HomeSkiareaID)
	}
	if opts.BirthmonthdayVisible != nil {
		req = req.BirthmonthdayVisible(*opts.BirthmonthdayVisible)
	}
	if opts.BirthyearVisible != nil {
		req = req.BirthyearVisible(*opts.BirthyearVisible)
	}
	if opts.BirthplaceVisible != nil {
		req = req.BirthplaceVisible(*opts.BirthplaceVisible)
	}
	if opts.IsShowSupporterLabel != nil {
		req = req.IsShowSupporterLabel(*opts.IsShowSupporterLabel)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "edit_user_new", "")
}

// EditUserGearOptions packages the optional fields for EditUserGear.
// Every field is a free-form text gear descriptor.
type EditUserGearOptions struct {
	// Snowboard description.
	GearBoard *string
	// Ski description.
	GearSki *string
	// Binding description.
	GearBinding *string
	// Boots description.
	GearBoots *string
	// Goggle description.
	GearGoggle *string
	// Gloves description.
	GearGloves *string
	// Wear description.
	GearWear *string
	// Pants description.
	GearPants *string
	// Pole description.
	GearPole *string
	// Head-gear description.
	GearHead *string
	// Other gear description.
	GearOther *string
}

// EditUserGear updates the caller's gear descriptors.
// Underlying op: GET /user/edit_user_gear (operationId: editUserGear).
func (s *UserService) EditUserGear(ctx context.Context, opts EditUserGearOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.EditUserGear(ctx)
	if opts.GearBoard != nil {
		req = req.GearBoard(*opts.GearBoard)
	}
	if opts.GearSki != nil {
		req = req.GearSki(*opts.GearSki)
	}
	if opts.GearBinding != nil {
		req = req.GearBinding(*opts.GearBinding)
	}
	if opts.GearBoots != nil {
		req = req.GearBoots(*opts.GearBoots)
	}
	if opts.GearGoggle != nil {
		req = req.GearGoggle(*opts.GearGoggle)
	}
	if opts.GearGloves != nil {
		req = req.GearGloves(*opts.GearGloves)
	}
	if opts.GearWear != nil {
		req = req.GearWear(*opts.GearWear)
	}
	if opts.GearPants != nil {
		req = req.GearPants(*opts.GearPants)
	}
	if opts.GearPole != nil {
		req = req.GearPole(*opts.GearPole)
	}
	if opts.GearHead != nil {
		req = req.GearHead(*opts.GearHead)
	}
	if opts.GearOther != nil {
		req = req.GearOther(*opts.GearOther)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "edit_user_gear", "")
}

// DeleteUserGear clears every gear descriptor on the caller's profile.
// Underlying op: GET /user/delete_user_gear (operationId: deleteUserGear).
func (s *UserService) DeleteUserGear(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.DeleteUserGear(ctx).Execute()
	return statusOrError(res, err, "delete_user_gear", "")
}

// ChangePassword updates the caller's password. The current spec does not
// model the new-password body for this op (the upstream client posts it as
// a body payload that this SDK does not yet surface); the facade is a thin
// pass-through pending a spec update.
// Underlying op: GET /user/change_pw (operationId: changePassword).
func (s *UserService) ChangePassword(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.ChangePassword(ctx).Execute()
	return statusOrError(res, err, "change_pw", "")
}

// UnlinkTwitter disconnects the caller's linked X (Twitter) account.
// Underlying op: GET /user/unlink_twitter (operationId: unlinkTwitter).
func (s *UserService) UnlinkTwitter(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.UnlinkTwitter(ctx).Execute()
	return statusOrError(res, err, "unlink_twitter", "")
}

// SaveAutopost toggles the cross-post-to-checkin flag. isAutopost is 0/1.
// Underlying op: GET /user/save_autopost (operationId: saveAutopost).
func (s *UserService) SaveAutopost(ctx context.Context, isAutopost int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.SaveAutopost(ctx).
		IsAutopost(isAutopost).
		Execute()
	return statusOrError(res, err, "save_autopost", "")
}

// --- mutations: push settings -----------------------------------------------

// UpdatePushSettingOptions packages every push opt-in flag for
// UpdatePushSetting. Each field is 0/1; omit to leave the server-side value
// untouched.
type UpdatePushSettingOptions struct {
	// "Someone liked your checkin" push.
	IsPushInterest *int32
	// "Someone commented on your checkin" push.
	IsPushComment *int32
	// "Someone followed you" push.
	IsPushFollow *int32
	// "Someone created a schedule" push.
	IsPushScheduleCreate *int32
	// "Someone joined your schedule" push.
	IsPushScheduleJoin *int32
	// General info / announcement push.
	IsPushInfo *int32
	// Geofencing checkin-alert push.
	IsPushCheckinAlert *int32
}

// UpdatePushSetting updates the caller's per-category push opt-ins.
// Underlying op: GET /user/update_push_setting (operationId: updatePushSetting).
func (s *UserService) UpdatePushSetting(ctx context.Context, opts UpdatePushSettingOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.UpdatePushSetting(ctx)
	if opts.IsPushInterest != nil {
		req = req.IsPushInterest(*opts.IsPushInterest)
	}
	if opts.IsPushComment != nil {
		req = req.IsPushComment(*opts.IsPushComment)
	}
	if opts.IsPushFollow != nil {
		req = req.IsPushFollow(*opts.IsPushFollow)
	}
	if opts.IsPushScheduleCreate != nil {
		req = req.IsPushScheduleCreate(*opts.IsPushScheduleCreate)
	}
	if opts.IsPushScheduleJoin != nil {
		req = req.IsPushScheduleJoin(*opts.IsPushScheduleJoin)
	}
	if opts.IsPushInfo != nil {
		req = req.IsPushInfo(*opts.IsPushInfo)
	}
	if opts.IsPushCheckinAlert != nil {
		req = req.IsPushCheckinAlert(*opts.IsPushCheckinAlert)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "update_push_setting", "")
}

// --- mutations: supporter / members card ------------------------------------

// UpdateSupporterStatusOptions packages the optional fields for
// UpdateSupporterStatus. The endpoint records an in-app supporter (paid)
// subscription receipt; the upstream client populates these from the
// platform billing layer.
type UpdateSupporterStatusOptions struct {
	// 0/1 — toggles the supporter flag on the account.
	IsSupporter *int32
	// Purchase date (YYYY-MM-DD).
	PurchaseDate *string
	// Plan selector (wire type: float32).
	PlanType *float32
	// Expiration date (YYYY-MM-DD).
	ExpirationDate *string
	// Platform-specific receipt payload (opaque).
	Receipt *string
	// Device type selector (e.g. 1=iOS, 2=Android).
	DeviceType *int32
	// Platform billing transaction id.
	TransactionID *string
	// Platform billing original-transaction id (auto-renewing subscriptions).
	OriginalTransactionID *string
}

// UpdateSupporterStatus records a supporter (paid) subscription receipt for
// the caller. Underlying op: GET /user/update_supporter_status
// (operationId: updateSupporterStatus).
func (s *UserService) UpdateSupporterStatus(ctx context.Context, opts UpdateSupporterStatusOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.UpdateSupporterStatus(ctx)
	if opts.IsSupporter != nil {
		req = req.IsSupporter(*opts.IsSupporter)
	}
	if opts.PurchaseDate != nil {
		req = req.PurchaseDate(*opts.PurchaseDate)
	}
	if opts.PlanType != nil {
		req = req.PlanType(*opts.PlanType)
	}
	if opts.ExpirationDate != nil {
		req = req.ExpirationDate(*opts.ExpirationDate)
	}
	if opts.Receipt != nil {
		req = req.Receipt(*opts.Receipt)
	}
	if opts.DeviceType != nil {
		req = req.DeviceType(*opts.DeviceType)
	}
	if opts.TransactionID != nil {
		req = req.TransactionId(*opts.TransactionID)
	}
	if opts.OriginalTransactionID != nil {
		req = req.OriginalTransactionId(*opts.OriginalTransactionID)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "update_supporter_status", "")
}

// CancelSupporterStatus clears the caller's supporter (paid) subscription
// state. isSupporter is the post-cancel value the caller wants to record
// (typically 0).
// Underlying op: GET /user/cancel_supporter_status
// (operationId: cancelSupporterStatus).
func (s *UserService) CancelSupporterStatus(ctx context.Context, isSupporter int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.CancelSupporterStatus(ctx).
		IsSupporter(isSupporter).
		Execute()
	return statusOrError(res, err, "cancel_supporter_status", "")
}

// AddMembersCard binds a members-card identifier to the caller's account.
// Underlying op: GET /user/add_members_card (operationId: addMembersCard).
func (s *UserService) AddMembersCard(ctx context.Context, cardID string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.AddMembersCard(ctx).
		CardId(cardID).
		Execute()
	return statusOrError(res, err, "add_members_card", "")
}

// CancelMembersCard unbinds the members-card identifier on the caller's
// account.
// Underlying op: GET /user/cancel_members_card
// (operationId: cancelMembersCard).
func (s *UserService) CancelMembersCard(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.UserAPI.CancelMembersCard(ctx).Execute()
	return statusOrError(res, err, "cancel_members_card", "")
}

// --- mutations: telemetry ----------------------------------------------------

// TrackAdAccessOptions packages the optional fields for TrackAdAccess.
type TrackAdAccessOptions struct {
	// Access kind selector (view / click / exit).
	Type *gen.AdAccessType
	// In-app shop ad id the event applies to.
	ShopAdID *int32
	// Stay-time payload (string-encoded, format varies by Type).
	StayTime *string
}

// TrackAdAccess records an ad-impression / ad-click telemetry event.
// Underlying op: GET /user/track_ad_access (operationId: trackAdAccess).
func (s *UserService) TrackAdAccess(ctx context.Context, opts TrackAdAccessOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.UserAPI.TrackAdAccess(ctx)
	if opts.Type != nil {
		req = req.Type_(*opts.Type)
	}
	if opts.ShopAdID != nil {
		req = req.ShopAdId(*opts.ShopAdID)
	}
	if opts.StayTime != nil {
		req = req.StayTime(*opts.StayTime)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "track_ad_access", "")
}
