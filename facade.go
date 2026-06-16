package yukiyama

import (
	"context"
	"errors"
	"strconv"

	"github.com/ekkx/yukiyama/gen"
)

// This file collects the high-level "do the obvious thing" facade methods that
// hang directly off *Client. They are thin wrappers around the auto-generated
// gen builders. Their sole responsibility is:
//
//   Provide ergonomic, session-aware entry points. user_id / token / version
//   are not set by facades because authTransport overwrites them on every
//   RoundTrip with the live session.
//
// Each facade is documented with its underlying OpenAPI operation id so the
// gen → facade mapping stays grep-able.
//
// Auth-state policy: every facade calls ensureSession at the top so that
// callers who explicitly disabled WithAutoLogin still get an actionable error
// rather than a "userId is required" surprise from the gen layer.

// ensureSession guarantees that c has a usable (user_id, token). If autoLogin
// is enabled it triggers Login when needed; otherwise it returns an actionable
// error pointing the caller at Login(ctx).
func (c *Client) ensureSession(ctx context.Context) error {
	if c.session.IsAuthenticated() {
		return nil
	}
	if c.cfg == nil || !c.cfg.autoLogin {
		return errors.New("yukiyama: not authenticated; call client.Login(ctx) first or enable WithAutoLogin(true)")
	}
	return c.Login(ctx)
}

// --- 認証 / ユーザ ------------------------------------------------------------

// GetMyProfile fetches the authenticated user's full profile.
// Underlying op: GET /user/get (operationId: getUserProfile).
//
// Wire-version caveat: the `version` query parameter on this endpoint is a
// *content schema* selector. The official client uses "3"; we pin the same
// value so transport auto-injection of APIVersionName cannot silently swap
// the server-side response shape.
func (c *Client) GetMyProfile(ctx context.Context) (*gen.UserGetResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.UserAPI.GetUserProfile(ctx).
		Id(c.CurrentUserID()).
		IsHome(0).
		Version("3").
		Execute()
	return res, err
}

// GetUserProfile fetches another user's profile by user_id.
// Underlying op: GET /user/get (operationId: getUserProfile).
//
// See GetMyProfile for the wire-version caveat — same endpoint, same pin.
func (c *Client) GetUserProfile(ctx context.Context, userID int32) (*gen.UserGetResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.UserAPI.GetUserProfile(ctx).
		Id(userID).
		IsHome(0).
		Version("3").
		Execute()
	return res, err
}

// Withdraw deletes the authenticated user's account. On success the in-memory
// session and the persistent SessionStore are both cleared, since the
// (user_id, token) pair is now permanently invalid on the server.
// Underlying op: GET /user/withdrawal (operationId: withdrawAccount).
func (c *Client) Withdraw(ctx context.Context) error {
	if err := c.ensureSession(ctx); err != nil {
		return err
	}
	_, _, err := c.api.UserAPI.WithdrawAccount(ctx).Execute()
	if err != nil {
		return err
	}
	// Server-side session is gone. Mirror that locally so subsequent calls
	// don't try to reuse a dead token. We deliberately call Logout (not
	// session.Clear) so the SessionStore is also wiped.
	c.Logout()
	return nil
}

// --- マスタ / ホーム ----------------------------------------------------------

// GetHomeData fetches the home-screen payload (carousels, popups, etc.).
// Underlying op: GET /common/get_home_data (operationId: getHomeData).
//
// Wire-version caveat: this endpoint's `version` query parameter is a
// *content schema* selector, not the SDK transport-injected APIVersionName.
// The official client uses "5"; we mirror that so the SDK never lets
// transport auto-injection overwrite it with the app version, which would
// return a different (and possibly older) schema.
func (c *Client) GetHomeData(ctx context.Context) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.CommonAPI.GetHomeData(ctx).
		Version("5").
		Execute()
	return res, err
}

// --- スキー場 ------------------------------------------------------------------

// GetSkiarea fetches detail metadata for a single ski area.
// Underlying op: GET /skiarea/get (operationId: getSkiarea).
//
// Wire shape: GET /skiarea/get?id=<skiareaID>. Despite the parameter being
// named `id` on the wire, it identifies the skiarea — see the godoc on the
// gen builder's Id setter.
func (c *Client) GetSkiarea(ctx context.Context, skiareaID int32) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.SkiareaAPI.GetSkiarea(ctx).
		Id(skiareaID).
		Execute()
	return res, err
}

// FindSkiareasByLocation searches ski areas by (lat, lng).
// Underlying op: GET /skiarea/find_by_location (operationId: findSkiareasByLocation).
//
// The gen builder names its parameters lat/lon and types them as float32;
// the public facade exposes float64 for caller convenience. The `id` query
// param on this op is operation-specific and conventionally carries the
// calling user's id — we plumb that explicitly. token/version are
// auth-injected by the transport.
func (c *Client) FindSkiareasByLocation(ctx context.Context, lat, lng float64) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.SkiareaAPI.FindSkiareasByLocation(ctx).
		Id(c.CurrentUserID()).
		Lat(float32(lat)).
		Lon(float32(lng)).
		Execute()
	return res, err
}

// FindSkiareasByKeyword searches ski areas by free-text keyword.
// Underlying op: GET /skiarea/find_by_keyword (operationId: findSkiareasByKeyword).
//
// The gen builder makes find_option optional; the facade omits it for the
// common "no filter" case. user_id/token/version are auth-injected.
func (c *Client) FindSkiareasByKeyword(ctx context.Context, keyword string) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.SkiareaAPI.FindSkiareasByKeyword(ctx).
		Keyword(keyword).
		Execute()
	return res, err
}

// GetSkiareaEvent fetches a single event by its event_id.
// Underlying op: GET /skiarea/get_event (operationId: getSkiareaEvent).
//
// Wire-naming caveat: despite the operation living under /skiarea/, the
// parameter is event_id — NOT skiarea_id. A live observation with
// event_id=1 returns a populated EventModel. The facade signature reflects
// this by taking eventID, not skiareaID.
func (c *Client) GetSkiareaEvent(ctx context.Context, eventID int32) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.SkiareaAPI.GetSkiareaEvent(ctx).
		EventId(eventID).
		Execute()
	return res, err
}

// Section conventions:
//   - Options-heavy endpoints take a `*Options` struct of pointer fields. nil
//     means "do not set" — the gen builder leaves the parameter absent, which
//     is the correct behavior for the upstream API's "string-empty-means-
//     unfiltered" sentinels.
//   - Endpoints whose wire `user_id`/`token`/`version` carries a non-session
//     semantic (CheckUserNameAvailable, getUnreadCount, etc.) set the value
//     explicitly on the gen builder so the transport's "caller wins"
//     injection (see transport.go injectAuth) does not preempt it.

// --- ユーザ (continued) -------------------------------------------------------

// CheckUserNameAvailable checks whether a desired public user_id (handle) is
// available for registration. Underlying op: GET /user/check_user_id_available
// (operationId: checkUserIdAvailable).
//
// Wire-name caveat: this endpoint's `user_id` query parameter is a *username
// string*, NOT the caller's numeric user_id. We explicitly populate it via
// `.UserId(userName)` so that authTransport's "fill if missing" semantics
// for user_id do not silently overwrite the caller's handle with the
// session's numeric id. Without that preemption the server would see e.g.
// `user_id=12345` and return the existence check for the caller's own account.
func (c *Client) CheckUserNameAvailable(ctx context.Context, userName string) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.UserAPI.CheckUserIdAvailable(ctx).
		UserId(userName).
		Execute()
	return res, err
}

// --- 通知 / バッジ ------------------------------------------------------------

// GetUnreadCount fetches the unread counters that decorate the home tab badges
// (notifications, messages, etc.).
// Underlying op: GET /common/get_unread_count (operationId: getUnreadCount).
//
// Wire-version caveat: this endpoint's `version` query parameter is a
// *content schema* selector. The official client uses "2"; we set it
// explicitly so transport auto-injection of APIVersionName cannot silently
// change the response shape.
func (c *Client) GetUnreadCount(ctx context.Context) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := c.api.CommonAPI.GetUnreadCount(ctx).
		Version("2").
		Execute()
	return res, err
}

// ListDistributionNotificationsOptions packages the optional query parameters
// for ListDistributionNotifications. nil pointer fields are not transmitted.
type ListDistributionNotificationsOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Offset into the result list (item count, not page count).
	Offset *int32
}

// ListDistributionNotifications fetches the user-targeted distribution
// notification feed (broadcasts, campaign messages, etc.).
// Underlying op: GET /common/get_notification_distribution_user
// (operationId: listDistributionNotifications).
//
// `cmd` is required by the wire (observed values include "get_list");
// callers pass it directly to keep the facade signature explicit about that
// requirement. Wire-version caveat: `version` is a content-schema selector
// pinned to "2", set here to prevent transport auto-injection.
func (c *Client) ListDistributionNotifications(ctx context.Context, cmd string, opts ListDistributionNotificationsOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := c.api.CommonAPI.ListDistributionNotifications(ctx).
		Cmd(cmd).
		Version("2")
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- チェックイン (continued) -------------------------------------------------

// CheckinTimelineOptions packages the optional query parameters for
// GetCheckinTimeline. nil pointer fields are not transmitted.
//
// Wire-type notes:
//   - TargetUser / TargetSkiarea / TargetTopics / FindOption are strings on
//     the wire even when they semantically carry numeric ids (empty string =
//     unfiltered); see the gen builder godoc.
//   - ApiVer is an int32 (observed value: 2).
type CheckinTimelineOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Sort order. Observed value: "new".
	Order *string
	// Optional group_id filter (empty string = unfiltered).
	Group *string
	// Optional target user_id filter (string-encoded, empty = unfiltered).
	TargetUser *string
	// Optional target skiarea_id filter (string-encoded, empty = unfiltered).
	TargetSkiarea *string
	// Optional target topics filter (string-encoded csv, empty = unfiltered).
	TargetTopics *string
	// JSON-encoded advanced search options (hash_tag etc).
	FindOption *string
	// Debugger flag (0/1).
	IsDebugger *int32
	// Internal API revision selector (wire param: api_ver). Observed value: 2.
	ApiVer *int32
}

// GetCheckinTimeline fetches the checkin feed (the "all" / group / user
// timelines visible on the checkin tab).
// Underlying op: GET /checkin/get_checkin_timeline (operationId: getCheckinTimeline).
//
// All filters are optional. user_id/token/version are filled by authTransport.
func (c *Client) GetCheckinTimeline(ctx context.Context, opts CheckinTimelineOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := c.api.CheckinAPI.GetCheckinTimeline(ctx)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	if opts.Order != nil {
		req = req.Order(*opts.Order)
	}
	if opts.Group != nil {
		req = req.Group(*opts.Group)
	}
	if opts.TargetUser != nil {
		req = req.TargetUser(*opts.TargetUser)
	}
	if opts.TargetSkiarea != nil {
		req = req.TargetSkiarea(*opts.TargetSkiarea)
	}
	if opts.TargetTopics != nil {
		req = req.TargetTopics(*opts.TargetTopics)
	}
	if opts.FindOption != nil {
		req = req.FindOption(*opts.FindOption)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	if opts.ApiVer != nil {
		req = req.ApiVer(*opts.ApiVer)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- スケジュール / 訪問履歴 --------------------------------------------------

// ListSchedulesOptions packages the optional query parameters for
// ListSchedules. nil pointer fields are not transmitted.
//
// Wire-naming caveat carries through (see ListSchedules godoc): the `Type`
// field maps to `type` on the wire — observed values include "list" (full
// list) and "month" (calendar view, requires From/To).
type ListSchedulesOptions struct {
	// View mode. Observed values: "list", "month".
	Type *string
	// Calendar window start date (YYYY-MM-DD). Empty when Type == "list".
	From *string
	// Calendar window end date (YYYY-MM-DD). Empty when Type == "list".
	To *string
}

// ListSchedules fetches the schedule entries (planned ride-outs) for a target
// user. Underlying op: GET /user/get_schedule (operationId: listSchedules).
//
// REVERSED-NAMING CAVEAT: unlike most yukiyama endpoints, this op's wire
// params `id` and `user_id` are *swapped*: `id` is the caller (authenticated
// user) and `user_id` is the target whose schedules are being queried. The
// facade hides that footgun — `targetUserID` is plumbed to `.UserId()` and
// `.Id()` is filled from CurrentUserID(). See the gen builder's setter
// godoc for the canonical statement of the semantics.
func (c *Client) ListSchedules(ctx context.Context, targetUserID int32, opts ListSchedulesOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := c.api.UserAPI.ListSchedules(ctx).
		Id(c.CurrentUserID()).
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
// Wire-param caveat: the official wire request carries `target_user_id=`
// to scope the query. The current OpenAPI spec only declares is_debugger,
// so the gen builder lacks a `.TargetUserId()` setter; the facade plumbs
// the value via the transport's per-request extra-query escape hatch
// (withExtraQuery) until the spec gains the param in a future iteration.
// Pass 0 to request your own visits (uses CurrentUserID).
func (c *Client) ListVisitedSkiareas(ctx context.Context, targetUserID int32, opts ListVisitedSkiareasOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = c.CurrentUserID()
	}
	ctx = withExtraQuery(ctx, map[string]string{
		"target_user_id": strconv.FormatInt(int64(targetUserID), 10),
	})
	req := c.api.UserAPI.ListVisitedSkiareas(ctx)
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
func (c *Client) ListVisitedSkiareasWithStats(ctx context.Context, targetUserID int32, opts ListVisitedSkiareasWithStatsOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = c.CurrentUserID()
	}
	req := c.api.UserAPI.ListVisitedSkiareasWithStats(ctx).
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

// Coverage rationale (see Client.Gen godoc): a handwritten facade is
// warranted when there is (a) a caller/target wire reversal to hide, or
// (b) a non-trivial Options bundle worth packaging. Endpoints without
// those concerns stay on Client.Gen() directly.

// CheckinTimelineWithTopicsOptions packages the optional query parameters
// for GetCheckinTimelineWithTopics. Identical to CheckinTimelineOptions
// except for the additional Cmd field (observed sub-command, e.g.
// "get_total" returns only the total count without the timeline payload).
//
// Wire-type notes match CheckinTimelineOptions — TargetUser / TargetSkiarea
// / TargetTopics / FindOption are strings on the wire even when they
// semantically carry numeric ids (empty string = unfiltered).
type CheckinTimelineWithTopicsOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Sort order. Observed value: "new".
	Order *string
	// Optional group_id filter (empty string = unfiltered).
	Group *string
	// Optional target user_id filter (string-encoded, empty = unfiltered).
	TargetUser *string
	// Optional target skiarea_id filter (string-encoded, empty = unfiltered).
	TargetSkiarea *string
	// Optional target topics filter (string-encoded csv, empty = unfiltered).
	TargetTopics *string
	// JSON-encoded advanced search options (hash_tag etc).
	FindOption *string
	// Debugger flag (0/1).
	IsDebugger *int32
	// Internal API revision selector (wire param: api_ver). Observed value: 2.
	ApiVer *int32
	// Optional sub-command. Observed value: "get_total" (returns only
	// total count, no timeline payload).
	Cmd *string
}

// GetCheckinTimelineWithTopics fetches the topics-augmented checkin feed
// (the "topics" tab variant of GetCheckinTimeline, used by the home screen
// for trending hashtag rollups).
// Underlying op: GET /checkin/get_timeline_with_topics
// (operationId: getCheckinTimelineWithTopics).
//
// All filters are optional. user_id/token/version are filled by authTransport.
func (c *Client) GetCheckinTimelineWithTopics(ctx context.Context, opts CheckinTimelineWithTopicsOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := c.api.CheckinAPI.GetCheckinTimelineWithTopics(ctx)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	if opts.Order != nil {
		req = req.Order(*opts.Order)
	}
	if opts.Group != nil {
		req = req.Group(*opts.Group)
	}
	if opts.TargetUser != nil {
		req = req.TargetUser(*opts.TargetUser)
	}
	if opts.TargetSkiarea != nil {
		req = req.TargetSkiarea(*opts.TargetSkiarea)
	}
	if opts.TargetTopics != nil {
		req = req.TargetTopics(*opts.TargetTopics)
	}
	if opts.FindOption != nil {
		req = req.FindOption(*opts.FindOption)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	if opts.ApiVer != nil {
		req = req.ApiVer(*opts.ApiVer)
	}
	if opts.Cmd != nil {
		req = req.Cmd(*opts.Cmd)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListCheckinHistoryOptions packages the optional query parameters for
// ListCheckinHistory. nil pointer fields are not transmitted.
type ListCheckinHistoryOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Season window start (YYYY-MM-DD).
	SeasonFrom *string
	// Season window end (YYYY-MM-DD).
	SeasonTo *string
	// Debugger flag (0/1).
	IsDebugger *int32
}

// ListCheckinHistory fetches the season-scoped checkin history for a target
// user. Underlying op: GET /checkin/get_checkin_history
// (operationId: listCheckinHistory).
//
// REVERSED-NAMING CAVEAT: like ListSchedules, this endpoint's wire param
// `id` carries the TARGET user_id (whose history to view), while the
// auth-injected `user_id` is the CALLER.
//
// The facade hides that footgun — `targetUserID` is plumbed to `.Id()` and
// the gen layer's `.UserId()` setter is left to authTransport.
//
// Pass 0 to view your own history (uses CurrentUserID).
func (c *Client) ListCheckinHistory(ctx context.Context, targetUserID int32, opts ListCheckinHistoryOptions) (*gen.CommonResponse, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = c.CurrentUserID()
	}
	req := c.api.CheckinAPI.ListCheckinHistory(ctx).
		Id(targetUserID)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	if opts.SeasonFrom != nil {
		req = req.SeasonFrom(*opts.SeasonFrom)
	}
	if opts.SeasonTo != nil {
		req = req.SeasonTo(*opts.SeasonTo)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}
