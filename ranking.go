package yukiyama

import (
	"context"

	gen "github.com/ekkx/yukiyama/gen"
)

// RankingService groups all /ranking/* operations with session-aware,
// wire-quirk-correcting ergonomics. Obtain it via client.Ranking.
//
// Methods follow the conventions documented at the top of
// skiarea.go.
type RankingService struct {
	c *Client
}

// --- read: master ------------------------------------------------------------

// GetRankingMasterOptions reserved for future optional params on
// GetRankingMaster. Empty today; kept so adding a param later is a
// non-breaking change.
type GetRankingMasterOptions struct{}

// GetRankingMaster fetches the ranking-master metadata: the canonical
// catalogue of ranking types, terms, categories, age buckets and gender
// buckets that the other ranking endpoints accept as filters.
// Underlying op: GET /ranking/get_ranking_master (operationId: getRankingMaster).
//
// user_id/token are injected by the transport.
func (s *RankingService) GetRankingMaster(ctx context.Context, _ GetRankingMasterOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.RankingAPI.GetRankingMaster(ctx).Execute()
	return res, err
}

// --- read: ranking list ------------------------------------------------------

// GetRankingListOptions packages the optional query parameters for
// RankingService.GetRankingList. Every filter is optional; an empty Options
// returns the upstream default ranking shape.
//
// Wire-type notes:
//   - RankingTarget, RankingCategoryAge and RankingCategoryGender are
//     strings on the wire even when they semantically carry numeric ids
//     (empty string = unfiltered). Their valid values live in the
//     ranking-master catalogue returned by GetRankingMaster.
type GetRankingListOptions struct {
	// Which ranking axis to fetch (distance, speed, check-in count).
	// Maps to gen.RANKINGTYPE_* constants.
	RankingType *gen.RankingType
	// Aggregation window (season, monthly, daily).
	// Maps to gen.RANKINGTERM_* constants.
	RankingTerm *gen.RankingTerm
	// Optional ranking_target filter (string-encoded id; empty = unfiltered).
	RankingTarget *string
	// Optional ranking_category filter. Maps to gen.RANKINGCATEGORY_* constants.
	RankingCategory *gen.RankingCategory
	// Optional age-bucket filter (string-encoded id; empty = unfiltered).
	RankingCategoryAge *string
	// Optional gender-bucket filter (string-encoded id; empty = unfiltered).
	RankingCategoryGender *string
	// Optional skiarea filter; restricts the ranking to a single ski area.
	RankingSkiareaID *int32
}

// GetRankingList fetches the global ranking under the given (type, term,
// category) filters. The response is a single composite ranking resource
// whose entries live inside it — the filters select one ranking, not a
// page of separate rankings — so the facade keeps the verbatim wire name
// "RankingList" instead of pluralizing to ListRankings.
// Underlying op: GET /ranking/get_ranking_list (operationId: getRankingList).
//
// user_id/token are injected by the transport.
func (s *RankingService) GetRankingList(ctx context.Context, opts GetRankingListOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.RankingAPI.GetRankingList(ctx)
	if opts.RankingType != nil {
		req = req.RankingType(*opts.RankingType)
	}
	if opts.RankingTerm != nil {
		req = req.RankingTerm(*opts.RankingTerm)
	}
	if opts.RankingTarget != nil {
		req = req.RankingTarget(*opts.RankingTarget)
	}
	if opts.RankingCategory != nil {
		req = req.RankingCategory(*opts.RankingCategory)
	}
	if opts.RankingCategoryAge != nil {
		req = req.RankingCategoryAge(*opts.RankingCategoryAge)
	}
	if opts.RankingCategoryGender != nil {
		req = req.RankingCategoryGender(*opts.RankingCategoryGender)
	}
	if opts.RankingSkiareaID != nil {
		req = req.RankingSkiareaId(*opts.RankingSkiareaID)
	}
	res, _, err := req.Execute()
	return res, err
}

// --- read: group ranking -----------------------------------------------------

// GetRankingGroupListOptions packages the optional query parameters for
// RankingService.GetRankingGroupList.
type GetRankingGroupListOptions struct {
	// Which ranking axis to fetch (distance, speed, check-in count).
	// Maps to gen.RANKINGTYPE_* constants.
	RankingType *gen.RankingType
	// Optional group_id filter; restricts the ranking to a single user group.
	GroupID *int32
	// Optional skiarea filter; restricts the ranking to a single ski area.
	SkiareaID *int32
}

// GetRankingGroupList fetches the per-group ranking (the leaderboard
// scoped to a user-group, e.g. friends-of-friends or a custom group). As
// with GetRankingList, the response is a single composite ranking
// resource — the facade keeps the verbatim wire name "RankingGroupList"
// instead of pluralizing to ListRankingsByGroup.
// Underlying op: GET /ranking/get_ranking_group_list
// (operationId: getRankingGroupList).
//
// user_id/token are injected by the transport.
func (s *RankingService) GetRankingGroupList(ctx context.Context, opts GetRankingGroupListOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	req := s.c.api.RankingAPI.GetRankingGroupList(ctx)
	if opts.RankingType != nil {
		req = req.RankingType(*opts.RankingType)
	}
	if opts.GroupID != nil {
		req = req.GroupId(*opts.GroupID)
	}
	if opts.SkiareaID != nil {
		req = req.SkiareaId(*opts.SkiareaID)
	}
	res, _, err := req.Execute()
	return res, err
}
