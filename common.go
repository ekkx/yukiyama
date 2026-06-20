package yukiyama

import (
	"context"
	"strconv"
	"strings"

	gen "github.com/ekkx/yukiyama/gen"
)

// CommonService groups all /common/* operations with session-aware,
// wire-quirk-correcting ergonomics. Obtain it via client.Common.
//
// Methods follow the conventions documented at the top of
// skiarea.go.
type CommonService struct {
	c *Client
}

// --- read: home / master ------------------------------------------------------

// GetHomeData fetches the home-screen payload (carousels, popups, etc.).
// Underlying op: GET /common/get_home_data (operationId: getHomeData).
//
// Wire-version caveat: this endpoint's `version` query parameter is a
// *content schema* selector, not the SDK transport-injected APIVersionName.
// The official client uses "5"; we mirror that so the SDK never lets
// transport auto-injection overwrite it with the app version, which would
// return a different (and possibly older) schema.
func (s *CommonService) GetHomeData(ctx context.Context) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetHomeData(ctx).
		Version("5").
		Execute()
	return res, err
}

// GetMasterOptions reserved for future optional params on GetMaster. Empty
// today; kept so adding a param later is a non-breaking change.
type GetMasterOptions struct{}

// GetMaster fetches the master catalogue (static reference data such as
// region lists, group lists, etc.) used to hydrate dropdowns and filters
// throughout the app.
// Underlying op: GET /common/get_master (operationId: getMaster).
//
// Returns *gen.MasterResponse (not *gen.CommonResponse) — this endpoint
// has its own response shape distinct from the generic envelope.
func (s *CommonService) GetMaster(ctx context.Context, _ GetMasterOptions) (*gen.MasterResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetMaster(ctx).Execute()
	return res, err
}

// --- read: notification badges -----------------------------------------------

// GetUnreadCount fetches the unread counters that decorate the home tab badges
// (notifications, messages, etc.).
// Underlying op: GET /common/get_unread_count (operationId: getUnreadCount).
//
// Wire-version caveat: this endpoint's `version` query parameter is a
// *content schema* selector. The official client uses "2"; we set it
// explicitly so transport auto-injection of APIVersionName cannot silently
// change the response shape.
func (s *CommonService) GetUnreadCount(ctx context.Context) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetUnreadCount(ctx).
		Version("2").
		Execute()
	return res, err
}

// --- read: distribution notifications (cmd-split) ----------------------------

// ListDistributionNotificationsOptions packages the optional query parameters
// for the cmd-split List*DistributionNotifications methods. nil pointer
// fields are not transmitted.
type ListDistributionNotificationsOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Offset into the result list (item count, not page count).
	Offset *int32
}

// ListGeneralDistributionNotifications fetches broadcast / campaign-style
// distribution notifications (the wire selects this slice with cmd="notification").
// Underlying op: GET /common/get_notification_distribution_user
// (operationId: listDistributionNotifications).
//
// Wire-version caveat: `version` is a content-schema selector pinned to "2",
// set here to prevent transport auto-injection.
func (s *CommonService) ListGeneralDistributionNotifications(ctx context.Context, opts ListDistributionNotificationsOptions) (*gen.CommonResponse, error) {
	return s.listDistributionNotifications(ctx, "notification", opts)
}

// ListLikeDistributionNotifications fetches the "someone liked your checkin"
// slice of the distribution feed (wire cmd="like").
//
// Wire-version caveat: `version` is pinned to "2"; see
// ListGeneralDistributionNotifications.
func (s *CommonService) ListLikeDistributionNotifications(ctx context.Context, opts ListDistributionNotificationsOptions) (*gen.CommonResponse, error) {
	return s.listDistributionNotifications(ctx, "like", opts)
}

// ListScheduleDistributionNotifications fetches the schedule-related slice
// of the distribution feed (wire cmd="schedule").
//
// Wire-version caveat: `version` is pinned to "2"; see
// ListGeneralDistributionNotifications.
func (s *CommonService) ListScheduleDistributionNotifications(ctx context.Context, opts ListDistributionNotificationsOptions) (*gen.CommonResponse, error) {
	return s.listDistributionNotifications(ctx, "schedule", opts)
}

func (s *CommonService) listDistributionNotifications(ctx context.Context, cmd string, opts ListDistributionNotificationsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.CommonAPI.ListDistributionNotifications(ctx).
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

// --- read: push notifications ------------------------------------------------

// ListPushNotificationsOptions packages the optional query parameters for
// ListPushNotifications. nil pointer fields are not transmitted.
type ListPushNotificationsOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Offset into the result list (item count, not page count).
	Offset *int32
}

// ListPushNotifications fetches the in-app push notification history for the
// authenticated user.
// Underlying op: GET /common/get_push_notification (operationId: listPushNotifications).
func (s *CommonService) ListPushNotifications(ctx context.Context, opts ListPushNotificationsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.CommonAPI.ListPushNotifications(ctx)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetPushNotificationDetail fetches detail for a single push notification by
// its id.
// Underlying op: GET /common/get_push_notification_detail
// (operationId: getPushNotificationDetail).
func (s *CommonService) GetPushNotificationDetail(ctx context.Context, pushNotificationID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetPushNotificationDetail(ctx).
		PushNotificationId(pushNotificationID).
		Execute()
	return res, err
}

// --- read: popup news --------------------------------------------------------

// GetPopupNewsOptions reserved for future optional params. Empty today; kept
// so adding a param later is a non-breaking change.
type GetPopupNewsOptions struct{}

// GetPopupNews fetches the modal-popup news items eligible to be shown to the
// authenticated user. Pair with the AckPopupNews* methods to record show /
// view / click telemetry.
// Underlying op: GET /common/get_popup_news (operationId: getPopupNews).
func (s *CommonService) GetPopupNews(ctx context.Context, _ GetPopupNewsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetPopupNews(ctx).Execute()
	return res, err
}

// --- read: friends now / friends schedule ------------------------------------

// GetFriendsNowSummaryOptions reserved for future optional params. Empty
// today; kept so adding a param later is a non-breaking change.
type GetFriendsNowSummaryOptions struct{}

// GetFriendsNowSummary fetches the home-screen "friends checked in right now"
// summary (the rolled-up tile shown above the full list).
// Underlying op: GET /common/get_friends_now (operationId: getFriendsNowSummary).
func (s *CommonService) GetFriendsNowSummary(ctx context.Context, _ GetFriendsNowSummaryOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.GetFriendsNowSummary(ctx).Execute()
	return res, err
}

// ListFriendsNowOptions packages the optional query parameters for
// ListFriendsNow.
type ListFriendsNowOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
}

// ListFriendsNow fetches the paginated list of friends who are currently
// checked in, scoped to a target user's friend graph.
// Underlying op: GET /common/get_friends_now_list (operationId: listFriendsNow).
//
// REVERSED-NAMING CAVEAT: as with ListSchedules / ListCheckinHistory, this
// endpoint's wire params `id` and `user_id` carry the *caller* and *target*
// respectively. `id` is the authenticated caller (filled from
// CurrentUserID()); `user_id` is the target whose friend graph is being
// queried.
//
// Pass 0 for targetUserID to query against your own friend graph
// (uses CurrentUserID).
func (s *CommonService) ListFriendsNow(ctx context.Context, targetUserID int32, opts ListFriendsNowOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.CommonAPI.ListFriendsNow(ctx).
		Id(s.c.CurrentUserID()).
		UserId(targetUserID)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListFriendsScheduleOptions packages the optional query parameters for
// ListFriendsSchedule.
type ListFriendsScheduleOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
}

// ListFriendsSchedule fetches the paginated list of upcoming planned
// schedules visible across a target user's friend graph.
// Underlying op: GET /common/get_friends_schedule_list
// (operationId: listFriendsSchedule).
//
// REVERSED-NAMING CAVEAT: identical shape to ListFriendsNow — wire `id`
// is the caller (filled from CurrentUserID()) and wire `user_id` is the
// target whose friend graph is being queried.
//
// Pass 0 for targetUserID to query against your own friend graph
// (uses CurrentUserID).
func (s *CommonService) ListFriendsSchedule(ctx context.Context, targetUserID int32, opts ListFriendsScheduleOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.CommonAPI.ListFriendsSchedule(ctx).
		Id(s.c.CurrentUserID()).
		UserId(targetUserID)
	if opts.Max != nil {
		req = req.Max(*opts.Max)
	}
	if opts.Offset != nil {
		req = req.Offset(*opts.Offset)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: misc static data --------------------------------------------------

// SearchBirthplacesOptions reserved for future optional params on
// SearchBirthplaces. Empty today; kept so adding a param later is a
// non-breaking change.
type SearchBirthplacesOptions struct{}

// SearchBirthplaces searches the birthplace master list by free-text keyword
// (used by profile-edit screens to populate the prefecture / city picker).
// Underlying op: GET /common/find_birthplace (operationId: findBirthplaces).
func (s *CommonService) SearchBirthplaces(ctx context.Context, keyword string, _ SearchBirthplacesOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.FindBirthplaces(ctx).
		Keyword(keyword).
		Execute()
	return res, err
}

// ListHolidaysOptions reserved for future optional params. Empty today;
// kept so adding a param later is a non-breaking change.
type ListHolidaysOptions struct{}

// ListHolidays fetches the national-holiday calendar used by the schedule
// view to render holiday markers.
// Underlying op: GET /common/get_holiday (operationId: listHolidays).
func (s *CommonService) ListHolidays(ctx context.Context, _ ListHolidaysOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CommonAPI.ListHolidays(ctx).Execute()
	return res, err
}

// --- mutations: notifications ------------------------------------------------

// MarkAllNotificationsRead clears the unread state on every in-app
// notification for the authenticated caller (matches the "mark all read"
// button on the notification list screen).
// Underlying op: GET /common/notification_all_view
// (operationId: markAllNotificationsRead).
func (s *CommonService) MarkAllNotificationsRead(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CommonAPI.MarkAllNotificationsRead(ctx).Execute()
	return statusOrError(res, err, "notification_all_view", "")
}

// --- mutations: popup news ack (cmd-split) -----------------------------------

// AckPopupNewsShown records that the given popup news items were displayed
// to the user. Underlying op: GET /common/popup_news (operationId:
// ackPopupNews) with cmd="show". Multiple IDs are concatenated on the wire
// as a comma-separated string.
func (s *CommonService) AckPopupNewsShown(ctx context.Context, popupNewsIDs []int32) error {
	return s.ackPopupNews(ctx, popupNewsIDs, "show")
}

// AckPopupNewsViewed records that the user viewed the given popup news items.
// Underlying op: GET /common/popup_news with cmd="view".
func (s *CommonService) AckPopupNewsViewed(ctx context.Context, popupNewsIDs []int32) error {
	return s.ackPopupNews(ctx, popupNewsIDs, "view")
}

// AckPopupNewsClicked records that the user clicked through on a popup news
// item. Underlying op: GET /common/popup_news with cmd="click".
func (s *CommonService) AckPopupNewsClicked(ctx context.Context, popupNewsIDs []int32) error {
	return s.ackPopupNews(ctx, popupNewsIDs, "click")
}

func (s *CommonService) ackPopupNews(ctx context.Context, popupNewsIDs []int32, cmd string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	parts := make([]string, len(popupNewsIDs))
	for i, id := range popupNewsIDs {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	res, _, err := s.c.api.CommonAPI.AckPopupNews(ctx).
		PopupNewsId(strings.Join(parts, ",")).
		Cmd(cmd).
		Execute()
	return statusOrError(res, err, "popup_news", cmd)
}

// --- mutations: client-side error report -------------------------------------

// SendJsErrorOptions packages the optional query parameters for
// SendJsError. All fields are optional on the wire; populate what you have.
type SendJsErrorOptions struct {
	// Source file where the error was raised.
	File *string
	// Line number within File.
	Line *float32
	// Client app version string (e.g. "10.3.3").
	AppVer *string
	// Platform identifier (observed values: "android", "ios").
	Platform *string
}

// SendJsError reports a client-side error to the server's diagnostic log.
// Underlying op: GET /common/send_js_error (operationId: sendJsError).
//
// `message` is the only semantically required field; the official client
// always sends it. The remaining fields land in SendJsErrorOptions because
// most callers won't have a meaningful File / Line / Platform tuple.
func (s *CommonService) SendJsError(ctx context.Context, message string, opts SendJsErrorOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CommonAPI.SendJsError(ctx).Message(message)
	if opts.File != nil {
		req = req.File(*opts.File)
	}
	if opts.Line != nil {
		req = req.Line(*opts.Line)
	}
	if opts.AppVer != nil {
		req = req.AppVer(*opts.AppVer)
	}
	if opts.Platform != nil {
		req = req.Platform(*opts.Platform)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "send_js_error", "")
}
