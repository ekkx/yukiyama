package yukiyama

import (
	"context"

	gen "github.com/ekkx/yukiyama/gen"
)

// Service pattern (replicated across all services)
//
// Each service is a thin, session-aware ergonomic layer over one generated
// XxxAPIService. The conventions below are the contract every service file
// (user.go, checkin.go, ...) follows.
//
//  1. Struct shape.
//     A service is just a back-pointer to its parent *Client:
//
//         type SkiareaService struct { c *Client }
//
//     It is created once in NewClient and exposed as a struct field
//     (client.Skiarea). No initialization beyond `&SkiareaService{c: c}`.
//
//  2. Reaching the generated builder.
//     All methods go through s.c.api.XAPI.Op(ctx). The Client.api field is
//     the canonical handle; do not re-cache service pointers on the service.
//
//  3. Required vs optional params.
//     Required wire params become positional args after ctx. Optional params
//     are bundled into an XxxOptions struct of pointer fields (nil = omit).
//     One or two optional params may stay positional if it reads more
//     naturally, but three or more should always migrate to Options.
//
//         func (s *SkiareaService) GetSkiarea(ctx, skiareaID int32, opts GetSkiareaOptions)
//
//  4. cmd-split (multi-verb on one endpoint).
//     Endpoints whose wire dispatch is a `cmd` string get one exported
//     method per cmd value, all routing through a single unexported helper.
//     The verbs are written as natural English (Like / Unlike, Follow /
//     Unfollow), not as wire literals.
//
//  5. Wire-quirk rename.
//     When a wire param is mis-named (typo, semantic reversal, sentinel
//     string-for-int), the public method takes the correct name and the
//     quirk is hidden inside the body. Document the original wire name in
//     the method godoc as "wire param: X" so consumers can grep for it.
//
//  6. ensureSession + statusOrError.
//     Every public method calls s.c.ensureSession(ctx) first. Read methods
//     return (*gen.X, error). Mutation methods that yield only a status
//     envelope return error only, via statusOrError.
//
//  7. Naming: verb + resource, AIP-aligned.
//     Method names are always <Verb><Resource> — the service struct
//     organizes operations but does not elide the resource. Standard verbs
//     (Get, List, Create, Update, Delete, Search) cover most cases;
//     custom verbs (Like, Unlike, Use, ...) take their natural English form.
//     Resource is singular for Get/Create/Update/Delete and plural for
//     List/Search.

// SkiareaService groups all /skiarea/* operations. Obtain it via client.Skiarea.
type SkiareaService struct {
	c *Client
}

// --- read: detail ------------------------------------------------------------

// GetSkiareaOptions packages the optional query parameters for
// SkiareaService.GetSkiarea.
type GetSkiareaOptions struct {
	// Set to "yes" to include evaluation aggregates in the response. Omit
	// (nil) for the lean shape.
	WithEvaluate *string
}

// GetSkiarea fetches detail metadata for a single ski area.
// Underlying op: GET /skiarea/get (operationId: getSkiarea).
//
// Wire shape: GET /skiarea/get?id=<skiareaID>. Despite the wire param being
// named `id`, it identifies the skiarea (not a user_id).
func (s *SkiareaService) GetSkiarea(ctx context.Context, skiareaID int32, opts GetSkiareaOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.GetSkiarea(ctx).Id(skiareaID)
	if opts.WithEvaluate != nil {
		req = req.WithEvaluate(*opts.WithEvaluate)
	}
	res, _, err := req.Execute()
	return res, err
}

// GetSkiareaStatusOptions reserved for future optional params on
// GetSkiareaStatus. Empty today; kept so adding a param later is a
// non-breaking change.
type GetSkiareaStatusOptions struct{}

// GetSkiareaStatus fetches the live operational status (open/closed, lift
// counts, snow data) for a single ski area.
// Underlying op: GET /skiarea/get_skiarea_status (operationId: getSkiareaStatus).
//
// Wire-quirk rename: the wire param is `skiara_id` (note the missing 'e'
// — observed verbatim in the official client). The facade exposes
// `skiareaID`; the typo'd setter is invoked internally.
func (s *SkiareaService) GetSkiareaStatus(ctx context.Context, skiareaID int32, _ GetSkiareaStatusOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.SkiareaAPI.GetSkiareaStatus(ctx).
		SkiaraId(skiareaID).
		Execute()
	return res, err
}

// GetSkiareaEvent fetches a single ski-area event by its event_id.
// Underlying op: GET /skiarea/get_event (operationId: getSkiareaEvent).
//
// Wire-naming caveat: despite the operation living under /skiarea/, the
// wire param is `event_id` — NOT a skiarea_id. A live observation with
// event_id=1 returns a populated EventModel.
func (s *SkiareaService) GetSkiareaEvent(ctx context.Context, eventID int32) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.SkiareaAPI.GetSkiareaEvent(ctx).
		EventId(eventID).
		Execute()
	return res, err
}

// --- read: search ------------------------------------------------------------

// SearchSkiareasByLocationOptions packages the optional query parameters for
// SkiareaService.SearchSkiareasByLocation.
type SearchSkiareasByLocationOptions struct {
	// Search radius in meters (observed: 50000 = 50 km). Omit for upstream default.
	Distance *int32
	// Debugger flag (0/1). Reveals unpublished skiareas if non-zero.
	IsDebugger *int32
}

// SearchSkiareasByLocation searches ski areas by (lat, lng) within an
// optional radius.
// Underlying op: GET /skiarea/find_by_location (operationId: findSkiareasByLocation).
//
// The generated builder names its float params lat/lon and types them as
// float32; the public facade takes float64 for caller convenience and
// converts. The wire's required `id` param is operation-specific and
// carries the calling user_id — the facade fills it from CurrentUserID()
// so callers do not have to.
func (s *SkiareaService) SearchSkiareasByLocation(ctx context.Context, lat, lng float64, opts SearchSkiareasByLocationOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.FindSkiareasByLocation(ctx).
		Id(s.c.CurrentUserID()).
		Lat(float32(lat)).
		Lon(float32(lng))
	if opts.Distance != nil {
		req = req.Distance(*opts.Distance)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// SearchSkiareasByKeywordOptions packages the optional query parameters for
// SkiareaService.SearchSkiareasByKeyword.
type SearchSkiareasByKeywordOptions struct {
	// JSON-encoded advanced search options.
	FindOption *string
	// Debugger flag (0/1).
	IsDebugger *int32
}

// SearchSkiareasByKeyword searches ski areas by free-text keyword.
// Underlying op: GET /skiarea/find_by_keyword (operationId: findSkiareasByKeyword).
//
// user_id/token/version are injected by the transport.
func (s *SkiareaService) SearchSkiareasByKeyword(ctx context.Context, keyword string, opts SearchSkiareasByKeywordOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.FindSkiareasByKeyword(ctx).Keyword(keyword)
	if opts.FindOption != nil {
		req = req.FindOption(*opts.FindOption)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// SearchSkiareaEventsOptions packages the optional query parameters for
// SkiareaService.SearchSkiareaEvents. All filters are optional; an empty
// Options returns the unfiltered first page.
type SearchSkiareaEventsOptions struct {
	// Free-text keyword filter.
	Keyword *string
	// JSON-encoded advanced search options.
	FindOption *string
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
}

// SearchSkiareaEvents searches ski-area events (campaigns, festivals, etc.).
// Underlying op: GET /skiarea/find_event (operationId: findSkiareaEvents).
//
// user_id/token/version are injected by the transport.
func (s *SkiareaService) SearchSkiareaEvents(ctx context.Context, opts SearchSkiareaEventsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.FindSkiareaEvents(ctx)
	if opts.Keyword != nil {
		req = req.Keyword(*opts.Keyword)
	}
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

// --- read: list --------------------------------------------------------------

// ListSkiareasOptions packages the optional query parameters for
// SkiareaService.ListSkiareas.
type ListSkiareasOptions struct {
	// Debugger flag (0/1). Reveals unpublished skiareas if non-zero.
	IsDebugger *int32
}

// ListSkiareas fetches the full catalogue of ski areas.
// Underlying op: GET /skiarea/get_all (operationId: listAllSkiareas).
func (s *SkiareaService) ListSkiareas(ctx context.Context, opts ListSkiareasOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.ListAllSkiareas(ctx)
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// ListSkiareaTopicsOptions packages the optional query parameters for
// SkiareaService.ListSkiareaTopics.
//
// Wire-type notes:
//   - Group / TargetSkiarea / FindOption are strings on the wire even when
//     they semantically carry numeric ids (empty string = unfiltered).
type ListSkiareaTopicsOptions struct {
	// Max page size. Pair with Offset.
	Max *int32
	// Page offset in items (not pages).
	Offset *int32
	// Sort order. Observed value: "new".
	Order *string
	// Optional group_id filter (empty string = unfiltered).
	Group *string
	// Optional target_skiarea filter (string-encoded id; empty = unfiltered).
	TargetSkiarea *string
	// JSON-encoded advanced search options.
	FindOption *string
	// Debugger flag (0/1).
	IsDebugger *int32
}

// ListSkiareaTopics fetches the community-topic feed for ski areas
// (per-skiarea posts in the "topics" tab of the official client).
// Underlying op: GET /skiarea/get_topics (operationId: listSkiareaTopics).
//
// user_id/token/version are injected by the transport.
func (s *SkiareaService) ListSkiareaTopics(ctx context.Context, opts ListSkiareaTopicsOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.ListSkiareaTopics(ctx)
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
	if opts.TargetSkiarea != nil {
		req = req.TargetSkiarea(*opts.TargetSkiarea)
	}
	if opts.FindOption != nil {
		req = req.FindOption(*opts.FindOption)
	}
	if opts.IsDebugger != nil {
		req = req.IsDebugger(*opts.IsDebugger)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: coupons -----------------------------------------------------------

// GetSkiareaCouponOptions packages the optional query parameters for
// SkiareaService.GetSkiareaCoupon.
type GetSkiareaCouponOptions struct {
	// Optional distribution channel id; binds the coupon to a specific
	// distribution context.
	CouponDistributionID *int32
}

// GetSkiareaCoupon fetches the coupon detail for a single coupon id.
// Underlying op: GET /skiarea/get_coupon (operationId: listSkiareaCoupons).
//
// Despite the generated operationId carrying a plural "list" shape, the
// response is per-coupon detail — keyed by couponID — so the facade
// exposes it as a singular Get.
func (s *SkiareaService) GetSkiareaCoupon(ctx context.Context, couponID int32, opts GetSkiareaCouponOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.ListSkiareaCoupons(ctx).CouponId(couponID)
	if opts.CouponDistributionID != nil {
		req = req.CouponDistributionId(*opts.CouponDistributionID)
	}
	res, _, err := req.Execute()
	return res, err
}

// UseSkiareaCouponOptions packages the optional query parameters for
// SkiareaService.UseSkiareaCoupon.
type UseSkiareaCouponOptions struct {
	// Optional distribution channel id; required when redeeming a coupon
	// surfaced through a specific distribution.
	CouponDistributionID *int32
	// Coupon type selector (observed values live on gen.CouponType).
	Type *gen.CouponType
}

// UseSkiareaCoupon marks a coupon as redeemed for the caller at a given ski area.
// Underlying op: GET /skiarea/use_coupon (operationId: useSkiareaCoupon).
//
// user_id/token are injected by the transport.
func (s *SkiareaService) UseSkiareaCoupon(ctx context.Context, skiareaID, couponID int32, opts UseSkiareaCouponOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.SkiareaAPI.UseSkiareaCoupon(ctx).
		SkiareaId(skiareaID).
		CouponId(couponID)
	if opts.CouponDistributionID != nil {
		req = req.CouponDistributionId(*opts.CouponDistributionID)
	}
	if opts.Type != nil {
		req = req.Type_(*opts.Type)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- mutations: topic interest (cmd-split) -----------------------------------

// LikeSkiareaTopic marks the caller as interested in a ski-area community topic.
// Underlying op: GET /skiarea/update_topics_interest
// (operationId: updateSkiareaTopicsInterest) with cmd="add".
func (s *SkiareaService) LikeSkiareaTopic(ctx context.Context, topicID int32) error {
	return s.updateTopicsInterest(ctx, topicID, "add")
}

// UnlikeSkiareaTopic removes the caller's interest from a ski-area topic.
// Underlying op: GET /skiarea/update_topics_interest with cmd="delete".
func (s *SkiareaService) UnlikeSkiareaTopic(ctx context.Context, topicID int32) error {
	return s.updateTopicsInterest(ctx, topicID, "delete")
}

func (s *SkiareaService) updateTopicsInterest(ctx context.Context, topicID int32, cmd string) error {
	if err := s.c.ensureSession(ctx); err != nil {
		return err
	}
	res, _, err := s.c.api.SkiareaAPI.UpdateSkiareaTopicsInterest(ctx).
		TopicsId(topicID).
		Cmd(cmd).
		Execute()
	return statusOrError(res, err, "update_topics_interest", cmd)
}
