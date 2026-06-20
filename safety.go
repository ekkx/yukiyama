package yukiyama

import (
	"context"

	gen "github.com/ekkx/yukiyama/gen"
)

// SafetyService groups all /safety/* operations with session-aware,
// wire-quirk-correcting ergonomics. Obtain it via client.Safety.
//
// Methods follow the conventions documented at the top of
// skiarea.go.
type SafetyService struct {
	c *Client
}

// --- read: history -----------------------------------------------------------

// ListMySafetyHistoryOptions reserved for future optional params on
// ListMySafetyHistory. Empty today; kept so adding a param later is a
// non-breaking change.
type ListMySafetyHistoryOptions struct{}

// ListMySafetyHistory fetches the caller's safety-report history (the list of
// safety entries the current user has submitted or is associated with).
// Underlying op: GET /safety/my_history (operationId: listMySafetyHistory).
//
// user_id/token are injected by the transport; no other wire params.
func (s *SafetyService) ListMySafetyHistory(ctx context.Context, _ ListMySafetyHistoryOptions) (*gen.CommonResponse, error) {
	if err := s.c.ensureSession(ctx); err != nil {
		return nil, err
	}
	res, _, err := s.c.api.SafetyAPI.ListMySafetyHistory(ctx).Execute()
	return res, err
}
