package yukiyama

import (
	"context"

	gen "github.com/ekkx/yukiyama/gen"
)

// CheckinService groups all /checkin/* operations with session-aware,
// wire-quirk-correcting ergonomics. Obtain it via client.Checkin.
//
// Methods follow the conventions documented at the top of
// skiarea.go.
type CheckinService struct {
	c *Client
}

// --- read: detail ------------------------------------------------------------

// GetCheckin fetches a single checkin by id.
// Underlying op: GET /checkin/get (operationId: getCheckin).
func (s *CheckinService) GetCheckin(ctx context.Context, checkinID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.GetCheckin(ctx).
		CheckinId(checkinID).
		Execute()
	return res, err
}

// GetCheckinGeodata fetches the GPS-trace payload (lat/lng samples) for a
// checkin scoped to a particular ski area.
// Underlying op: GET /checkin/get_geodata (operationId: getCheckinGeodata).
//
// Token auth is not required by the underlying endpoint; user_id is filled
// by the transport.
func (s *CheckinService) GetCheckinGeodata(ctx context.Context, skiareaID, checkinID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.GetCheckinGeodata(ctx).
		SkiareaId(skiareaID).
		CheckinId(checkinID).
		Execute()
	return res, err
}

// CheckIsCheckedInOptions reserved for future optional params on
// IsCheckedIn. Empty today; kept so adding a param later is a
// non-breaking change.
type CheckIsCheckedInOptions struct{}

// IsCheckedIn reports whether the caller currently has an open checkin at
// the given ski area (the wire-level boolean is wrapped in CommonResponse).
// Underlying op: GET /checkin/get_is_checkin (operationId: checkIsCheckedIn).
//
// Naming exception: this method is a predicate ("is X?") rather than a
// noun-shaped resource read, so the verb-then-resource convention used
// elsewhere in this service does not apply. The natural English idiom
// `IsCheckedIn` is preserved.
func (s *CheckinService) IsCheckedIn(ctx context.Context, skiareaID int32, _ CheckIsCheckedInOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.CheckIsCheckedIn(ctx).
		SkiareaId(skiareaID).
		Execute()
	return res, err
}

// GetRideForGoodInfo fetches the RideForGood campaign metadata.
// Underlying op: GET /checkin/get_rideforgood (operationId: getRideForGoodInfo).
//
// This endpoint takes no parameters; the transport's user_id/token
// injection setters are absent on the generated builder.
func (s *CheckinService) GetRideForGoodInfo(ctx context.Context) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.GetRideForGoodInfo(ctx).Execute()
	return res, err
}

// --- read: timeline / history ------------------------------------------------

// GetCheckinTimelineOptions packages the optional query parameters for
// CheckinService.GetCheckinTimeline. nil pointer fields are not transmitted.
//
// Wire-type notes:
//   - TargetUser / TargetSkiarea / TargetTopics / FindOption are strings on
//     the wire even when they semantically carry numeric ids (empty string =
//     unfiltered); see the gen builder godoc.
//   - ApiVer is an int32 (observed value: 2).
type GetCheckinTimelineOptions struct {
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
// Underlying op: GET /checkin/get_timeline (operationId: getCheckinTimeline).
//
// All filters are optional. user_id/token/version are filled by the
// transport.
func (s *CheckinService) GetCheckinTimeline(ctx context.Context, opts GetCheckinTimelineOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.CheckinAPI.GetCheckinTimeline(ctx)
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

// GetCheckinTimelineWithTopicsOptions packages the optional query parameters
// for CheckinService.GetCheckinTimelineWithTopics. Identical to
// GetCheckinTimelineOptions except for the additional Cmd field (observed
// sub-command, e.g. "get_total" returns only the total count without the
// timeline payload).
//
// Wire-type notes match GetCheckinTimelineOptions — TargetUser / TargetSkiarea
// / TargetTopics / FindOption are strings on the wire even when they
// semantically carry numeric ids (empty string = unfiltered).
type GetCheckinTimelineWithTopicsOptions struct {
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

// GetCheckinTimelineWithTopics fetches the topics-augmented checkin feed (the
// "topics" tab variant of GetCheckinTimeline, used by the home screen for
// trending hashtag rollups).
// Underlying op: GET /checkin/get_timeline_with_topics
// (operationId: getCheckinTimelineWithTopics).
//
// All filters are optional. user_id/token/version are filled by the
// transport.
func (s *CheckinService) GetCheckinTimelineWithTopics(ctx context.Context, opts GetCheckinTimelineWithTopicsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.CheckinAPI.GetCheckinTimelineWithTopics(ctx)
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
// CheckinService.ListCheckinHistory. nil pointer fields are not transmitted.
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
// user.
// Underlying op: GET /checkin/get_checkin_history
// (operationId: listCheckinHistory).
//
// REVERSED-NAMING CAVEAT: this endpoint's wire param `id` carries the
// TARGET user_id (whose history to view), while the auth-injected
// `user_id` is the CALLER.
//
// The facade hides that footgun — `targetUserID` is plumbed to `.Id()` and
// the gen layer's `.UserId()` setter is left to the transport. Pass 0 to
// view your own history (uses CurrentUserID).
func (s *CheckinService) ListCheckinHistory(ctx context.Context, targetUserID int32, opts ListCheckinHistoryOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	if targetUserID == 0 {
		targetUserID = s.c.CurrentUserID()
	}
	req := s.c.api.CheckinAPI.ListCheckinHistory(ctx).
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

// ListCheckinInterestUsersOptions packages the optional query parameters for
// CheckinService.ListCheckinInterestUsers.
//
// Exactly one of CheckinID or TopicsID is expected by the server: the
// endpoint pivots its response between "users who liked this checkin"
// and "users who liked this topic" depending on which is set.
type ListCheckinInterestUsersOptions struct {
	// Lookup users who liked the given checkin id.
	CheckinID *int32
	// Lookup users who liked the given topics id.
	TopicsID *int32
}

// ListCheckinInterestUsers fetches the list of users who tapped "interested"
// (i.e. liked) the given checkin or topic.
// Underlying op: GET /checkin/get_interest_user_list
// (operationId: listCheckinInterestUsers).
func (s *CheckinService) ListCheckinInterestUsers(ctx context.Context, opts ListCheckinInterestUsersOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.CheckinAPI.ListCheckinInterestUsers(ctx)
	if opts.CheckinID != nil {
		req = req.CheckinId(*opts.CheckinID)
	}
	if opts.TopicsID != nil {
		req = req.TopicsId(*opts.TopicsID)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: groups ------------------------------------------------------------

// ListCheckinGroups fetches the checkin groups active at a given ski area
// (the "group" tab in the official client).
// Underlying op: GET /checkin/get_group (operationId: listCheckinGroups).
func (s *CheckinService) ListCheckinGroups(ctx context.Context, skiareaID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.ListCheckinGroups(ctx).
		SkiareaId(skiareaID).
		Execute()
	return res, err
}

// GetCheckinGroup fetches a checkin group's detail payload by id.
// Underlying op: GET /checkin/get_group_detail
// (operationId: getCheckinGroupDetail).
func (s *CheckinService) GetCheckinGroup(ctx context.Context, groupID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.GetCheckinGroupDetail(ctx).
		GroupId(groupID).
		Execute()
	return res, err
}

// ListCheckinGroupMemberLocations fetches the live (lat, lng) of each member
// in a checkin group — used by the group map view.
// Underlying op: GET /checkin/get_group_member_location
// (operationId: listCheckinGroupMemberLocations).
func (s *CheckinService) ListCheckinGroupMemberLocations(ctx context.Context, groupID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.ListCheckinGroupMemberLocations(ctx).
		GroupId(groupID).
		Execute()
	return res, err
}

// --- read: lottery -----------------------------------------------------------

// DrawCheckinLottery executes a lottery draw tied to the given checkin and
// returns the prize (or an empty result if the user did not win).
// Underlying op: GET /checkin/lottery (operationId: drawCheckinLottery).
//
// Despite being read-shaped on the wire (GET), the call has mutation-like
// side effects on the server (one draw per (user, lottery, checkin) tuple).
func (s *CheckinService) DrawCheckinLottery(ctx context.Context, lotteryID, checkinID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.CheckinAPI.DrawCheckinLottery(ctx).
		LotteryId(lotteryID).
		CheckinId(checkinID).
		Execute()
	return res, err
}

// --- mutations: checkin lifecycle --------------------------------------------

// StartCheckinOptions packages the optional query parameters for
// CheckinService.StartCheckin. nil pointer fields are not transmitted.
type StartCheckinOptions struct {
	// Visibility flag (0=private, 1=public). Observed default behavior:
	// upstream treats absent as public.
	IsOpen *int32
	// Auto-post toggle (0/1) — when set, the server cross-posts the
	// checkin to the timeline as soon as it begins.
	IsAutopost *int32
	// Free-text weather code (string on the wire).
	Weather *string
	// Free-text snow-state code (string on the wire).
	SnowState *string
	// Free-text course-state code (string on the wire). Spelled
	// "cource_state" on the wire — observed verbatim in the official
	// client.
	CourceState *string
	// Free-text temperature reading (string on the wire to allow units).
	Temperature *string
	// Free-text snow-accumulation reading (string on the wire).
	SnowAccumulation *string
}

// StartCheckin opens a new checkin at the given ski area.
// Underlying op: GET /checkin/start (operationId: startCheckin).
func (s *CheckinService) StartCheckin(ctx context.Context, skiareaID int32, opts StartCheckinOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.StartCheckin(ctx).
		SkiareaId(skiareaID)
	if opts.IsOpen != nil {
		req = req.IsOpen(*opts.IsOpen)
	}
	if opts.IsAutopost != nil {
		req = req.IsAutopost(*opts.IsAutopost)
	}
	if opts.Weather != nil {
		req = req.Weather(*opts.Weather)
	}
	if opts.SnowState != nil {
		req = req.SnowState(*opts.SnowState)
	}
	if opts.CourceState != nil {
		req = req.CourceState(*opts.CourceState)
	}
	if opts.Temperature != nil {
		req = req.Temperature(*opts.Temperature)
	}
	if opts.SnowAccumulation != nil {
		req = req.SnowAccumulation(*opts.SnowAccumulation)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "start", "")
}

// EndCheckinOptions packages the optional query parameters for
// CheckinService.EndCheckin.
type EndCheckinOptions struct {
	// Top speed (m/s on the wire) recorded during the session.
	MaxSpeed *float32
	// Total slide distance (meters on the wire) recorded during the session.
	SlideDistance *float32
}

// EndCheckin closes an open checkin.
// Underlying op: GET /checkin/end (operationId: endCheckin).
func (s *CheckinService) EndCheckin(ctx context.Context, checkinID int32, opts EndCheckinOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.EndCheckin(ctx).
		CheckinId(checkinID)
	if opts.MaxSpeed != nil {
		req = req.MaxSpeed(*opts.MaxSpeed)
	}
	if opts.SlideDistance != nil {
		req = req.SlideDistance(*opts.SlideDistance)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "end", "")
}

// UpdateCheckin is a placeholder mutation that accepts no parameters — used
// by the official client to ping the server mid-session so it can refresh
// its idea of "currently checked in". The generated builder exposes no
// setters; user_id/token are not auto-injected for this endpoint.
// Underlying op: POST /checkin/update (operationId: updateCheckin).
func (s *CheckinService) UpdateCheckin(ctx context.Context) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckin(ctx).Execute()
	return statusOrError(res, err, "update", "")
}

// PostOfflineCheckinOptions packages the optional form-body parameters for
// CheckinService.PostOfflineCheckin. nil pointer fields are not transmitted.
type PostOfflineCheckinOptions struct {
	// Coordinate accuracy threshold (meters) for grouping the GPS samples.
	OpenRange *float32
	// Free-text caption.
	Title *string
	// Layout selector for the rendered timeline card (string code).
	TimelineLayout *string
	// Optional YouTube URL to embed in the checkin card.
	YoutubeUrl *string
}

// PostOfflineCheckin submits a checkin that was recorded offline and is
// being uploaded after the fact.
// Underlying op: POST /checkin/offpost (operationId: postOfflineCheckin).
//
// Unlike the rest of /checkin/*, this is a form-body POST.
func (s *CheckinService) PostOfflineCheckin(ctx context.Context, skiareaID int32, opts PostOfflineCheckinOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.PostOfflineCheckin(ctx).
		UserId(s.c.CurrentUserID()).
		Token(s.c.CurrentToken()).
		SkiareaId(skiareaID)
	if opts.OpenRange != nil {
		req = req.OpenRange(*opts.OpenRange)
	}
	if opts.Title != nil {
		req = req.Title(*opts.Title)
	}
	if opts.TimelineLayout != nil {
		req = req.TimelineLayout(*opts.TimelineLayout)
	}
	if opts.YoutubeUrl != nil {
		req = req.YoutubeUrl(*opts.YoutubeUrl)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "offpost", "")
}

// EditCheckinGpslogOptions packages the optional query parameters for
// CheckinService.EditCheckinGpslog.
type EditCheckinGpslogOptions struct {
	// Ski area id that the gpslog belongs to.
	SkiareaID *int32
	// Start-position update (string-encoded "lat,lng" on the wire).
	Start *string
	// Goal-positions update (string-encoded csv of "lat,lng" on the wire).
	Goals *string
}

// EditCheckinGpslog updates the start/goal coordinates of an existing
// checkin's GPS trace (used by the post-ride trace cleanup UI).
// Underlying op: GET /checkin/edit_gpslog (operationId: editCheckinGpslog).
func (s *CheckinService) EditCheckinGpslog(ctx context.Context, checkinID int32, opts EditCheckinGpslogOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.EditCheckinGpslog(ctx).
		CheckinId(checkinID)
	if opts.SkiareaID != nil {
		req = req.SkiareaId(*opts.SkiareaID)
	}
	if opts.Start != nil {
		req = req.Start(*opts.Start)
	}
	if opts.Goals != nil {
		req = req.Goals(*opts.Goals)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "edit_gpslog", "")
}

// DeleteCheckin removes a checkin authored by the caller.
// Underlying op: GET /checkin/delete (operationId: deleteCheckin).
func (s *CheckinService) DeleteCheckin(ctx context.Context, checkinID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.DeleteCheckin(ctx).
		CheckinId(checkinID).
		Execute()
	return statusOrError(res, err, "delete", "")
}

// DeleteCheckinMarker removes a checkin marker (a map pin annotation
// attached to a checkin).
// Underlying op: GET /checkin/marker_delete (operationId: deleteCheckinMarker).
func (s *CheckinService) DeleteCheckinMarker(ctx context.Context, markerID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.DeleteCheckinMarker(ctx).
		MarkerId(markerID).
		Execute()
	return statusOrError(res, err, "marker_delete", "")
}

// --- mutations: groups -------------------------------------------------------

// JoinCheckinGroupOptions packages the optional query parameters for
// CheckinService.JoinCheckinGroup.
type JoinCheckinGroupOptions struct {
	// Optional password for protected groups.
	Password *string
}

// JoinCheckinGroup joins the caller to a checkin group.
// Underlying op: GET /checkin/join_group (operationId: joinCheckinGroup).
func (s *CheckinService) JoinCheckinGroup(ctx context.Context, groupID int32, opts JoinCheckinGroupOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.JoinCheckinGroup(ctx).
		GroupId(groupID)
	if opts.Password != nil {
		req = req.Password(*opts.Password)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "join_group", "")
}

// ExitCheckinGroupOptions packages the optional query parameters for
// CheckinService.ExitCheckinGroup.
type ExitCheckinGroupOptions struct {
	// Optional password for protected groups (some upstream group types
	// require the leave operation to re-prove membership).
	Password *string
}

// ExitCheckinGroup removes the caller from a checkin group.
// Underlying op: GET /checkin/exit_group (operationId: exitCheckinGroup).
func (s *CheckinService) ExitCheckinGroup(ctx context.Context, groupID int32, opts ExitCheckinGroupOptions) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	req := s.c.api.CheckinAPI.ExitCheckinGroup(ctx).
		GroupId(groupID)
	if opts.Password != nil {
		req = req.Password(*opts.Password)
	}
	res, _, err := req.Execute()
	return statusOrError(res, err, "exit_group", "")
}

// DeleteCheckinGroup removes a checkin group entirely (owner-only operation).
// Underlying op: GET /checkin/delete_group (operationId: deleteCheckinGroup).
func (s *CheckinService) DeleteCheckinGroup(ctx context.Context, groupID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.DeleteCheckinGroup(ctx).
		GroupId(groupID).
		Execute()
	return statusOrError(res, err, "delete_group", "")
}

// DeleteCheckinGroupMember removes a single member's trektrack entry from a
// checkin group (the wire op-name reflects the underlying entity, not
// the user model).
// Underlying op: GET /checkin/delete_group_member_trektrack
// (operationId: deleteCheckinGroupMember).
func (s *CheckinService) DeleteCheckinGroupMember(ctx context.Context, groupMemberTrektrackID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.DeleteCheckinGroupMember(ctx).
		GroupMemberTrektrackId(groupMemberTrektrackID).
		Execute()
	return statusOrError(res, err, "delete_group_member_trektrack", "")
}

// --- mutations: interest (cmd-split) -----------------------------------------

// LikeCheckin marks the caller as "interested in" (i.e. likes) the given
// checkin. Underlying op: GET /checkin/update_interest
// (operationId: updateCheckinInterest) with cmd="add".
//
// Equivalent to tapping the heart icon on a timeline entry. Idempotent
// on the wire — re-liking a post that is already liked returns
// status=true.
func (s *CheckinService) LikeCheckin(ctx context.Context, checkinID int32) error {
	return s.updateInterest(ctx, checkinID, "add")
}

// UnlikeCheckin removes the caller's interest from (i.e. unlikes) the given
// checkin. Underlying op: GET /checkin/update_interest with cmd="delete".
func (s *CheckinService) UnlikeCheckin(ctx context.Context, checkinID int32) error {
	return s.updateInterest(ctx, checkinID, "delete")
}

func (s *CheckinService) updateInterest(ctx context.Context, checkinID int32, cmd string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckinInterest(ctx).
		CheckinId(checkinID).
		Cmd(cmd).
		Execute()
	return statusOrError(res, err, "update_interest", cmd)
}

// --- mutations: comment (cmd-split) ------------------------------------------

// AddCheckinComment posts a new top-level comment on the given checkin.
// Underlying op: GET /checkin/update_comment
// (operationId: updateCheckinComment) with cmd="add".
func (s *CheckinService) AddCheckinComment(ctx context.Context, checkinID int32, comment string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckinComment(ctx).
		CheckinId(checkinID).
		Comment(comment).
		Cmd("add").
		Execute()
	return statusOrError(res, err, "update_comment", "add")
}

// EditCheckinComment edits an existing comment on a checkin.
// Underlying op: GET /checkin/update_comment with cmd="update".
func (s *CheckinService) EditCheckinComment(ctx context.Context, checkinID, commentID int32, comment string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckinComment(ctx).
		CheckinId(checkinID).
		CommentId(commentID).
		Comment(comment).
		Cmd("update").
		Execute()
	return statusOrError(res, err, "update_comment", "update")
}

// DeleteCheckinComment removes a comment from a checkin.
// Underlying op: GET /checkin/update_comment with cmd="delete".
func (s *CheckinService) DeleteCheckinComment(ctx context.Context, checkinID, commentID int32) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckinComment(ctx).
		CheckinId(checkinID).
		CommentId(commentID).
		Cmd("delete").
		Execute()
	return statusOrError(res, err, "update_comment", "delete")
}

// ReplyCheckinComment posts a reply targeting an existing comment.
// Underlying op: GET /checkin/update_comment with cmd="reply".
//
// replyTargetCommentID identifies the comment being replied to within
// the thread.
func (s *CheckinService) ReplyCheckinComment(ctx context.Context, checkinID, replyTargetCommentID int32, comment string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.CheckinAPI.UpdateCheckinComment(ctx).
		CheckinId(checkinID).
		ReplyTargetCommentId(replyTargetCommentID).
		Comment(comment).
		Cmd("reply").
		Execute()
	return statusOrError(res, err, "update_comment", "reply")
}
