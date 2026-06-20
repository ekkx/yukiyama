package yukiyama

import (
	"testing"

	gen "github.com/ekkx/yukiyama/gen"
)

// TestSkiareaService_Wired is a construction smoke test: NewClient must
// initialize the Skiarea service field, and the back-pointer must point at
// the same Client. Catches any future regression where someone adds a new
// service struct but forgets to wire it in NewClient.
func TestSkiareaService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Skiarea == nil {
		t.Fatal("client.Skiarea is nil; NewClient did not initialize SkiareaService")
	}
	if c.Skiarea.c != c {
		t.Errorf("client.Skiarea.c = %p, want %p (back-pointer mis-wired)", c.Skiarea.c, c)
	}
}

// TestSkiareaService_OptionsConstruction is a compile-time guard. Each
// Options struct is constructed with every field populated so that any
// rename or type change in the struct definitions surfaces at `go test`
// time rather than at a downstream caller.
func TestSkiareaService_OptionsConstruction(t *testing.T) {
	var (
		s          = "x"
		i32  int32 = 1
		ctyp       = gen.COUPONTYPE_CHECKIN
	)

	_ = GetSkiareaOptions{
		WithEvaluate: &s,
	}
	_ = GetSkiareaStatusOptions{}
	_ = SearchSkiareasByLocationOptions{
		Distance:   &i32,
		IsDebugger: &i32,
	}
	_ = SearchSkiareasByKeywordOptions{
		FindOption: &s,
		IsDebugger: &i32,
	}
	_ = SearchSkiareaEventsOptions{
		Keyword:    &s,
		FindOption: &s,
		Max:        &i32,
		Offset:     &i32,
	}
	_ = ListSkiareasOptions{
		IsDebugger: &i32,
	}
	_ = ListSkiareaTopicsOptions{
		Max:           &i32,
		Offset:        &i32,
		Order:         &s,
		Group:         &s,
		TargetSkiarea: &s,
		FindOption:    &s,
		IsDebugger:    &i32,
	}
	_ = GetSkiareaCouponOptions{
		CouponDistributionID: &i32,
	}
	_ = UseSkiareaCouponOptions{
		CouponDistributionID: &i32,
		Type:                 &ctyp,
	}
}
