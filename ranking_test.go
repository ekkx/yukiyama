package yukiyama

import (
	"testing"

	gen "github.com/ekkx/yukiyama/gen"
)

// TestRankingService_Wired is a construction smoke test: NewClient must
// initialize the Ranking service field, and the back-pointer must point at
// the same Client. Catches any future regression where someone adds a new
// service struct but forgets to wire it in NewClient.
func TestRankingService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Ranking == nil {
		t.Fatal("client.Ranking is nil; NewClient did not initialize RankingService")
	}
	if c.Ranking.c != c {
		t.Errorf("client.Ranking.c = %p, want %p (back-pointer mis-wired)", c.Ranking.c, c)
	}
}

// TestRankingService_OptionsConstruction is a compile-time guard. Each
// Options struct is constructed with every field populated so that any
// rename or type change in the struct definitions surfaces at `go test`
// time rather than at a downstream caller.
func TestRankingService_OptionsConstruction(t *testing.T) {
	var (
		s          = "x"
		i32  int32 = 1
		rtyp       = gen.RANKINGTYPE_DISTANCE
		rtrm       = gen.RANKINGTERM_SEASON
		rcat       = gen.RankingCategory(0)
	)

	_ = GetRankingMasterOptions{}
	_ = GetRankingListOptions{
		RankingType:           &rtyp,
		RankingTerm:           &rtrm,
		RankingTarget:         &s,
		RankingCategory:       &rcat,
		RankingCategoryAge:    &s,
		RankingCategoryGender: &s,
		RankingSkiareaID:      &i32,
	}
	_ = GetRankingGroupListOptions{
		RankingType: &rtyp,
		GroupID:     &i32,
		SkiareaID:   &i32,
	}
}
