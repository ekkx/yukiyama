package yukiyama

import (
	"testing"
)

// TestCommonService_Wired is a construction smoke test: NewClient must
// initialize the Common service field, and the back-pointer must point at
// the same Client. Catches any future regression where someone adds a new
// service struct but forgets to wire it in NewClient.
func TestCommonService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Common == nil {
		t.Fatal("client.Common is nil; NewClient did not initialize CommonService")
	}
	if c.Common.c != c {
		t.Errorf("client.Common.c = %p, want %p (back-pointer mis-wired)", c.Common.c, c)
	}
}

// TestCommonService_OptionsConstruction is a compile-time guard. Each
// Options struct is constructed with every field populated so that any
// rename or type change in the struct definitions surfaces at `go test`
// time rather than at a downstream caller.
func TestCommonService_OptionsConstruction(t *testing.T) {
	var (
		s         = "x"
		i32 int32 = 1
		f32       = float32(1)
	)

	_ = GetMasterOptions{}
	_ = ListDistributionNotificationsOptions{
		Max:    &i32,
		Offset: &i32,
	}
	_ = ListPushNotificationsOptions{
		Max:    &i32,
		Offset: &i32,
	}
	_ = GetPopupNewsOptions{}
	_ = GetFriendsNowSummaryOptions{}
	_ = ListFriendsNowOptions{
		Max:    &i32,
		Offset: &i32,
	}
	_ = ListFriendsScheduleOptions{
		Max:    &i32,
		Offset: &i32,
	}
	_ = SearchBirthplacesOptions{}
	_ = ListHolidaysOptions{}
	_ = SendJsErrorOptions{
		File:     &s,
		Line:     &f32,
		AppVer:   &s,
		Platform: &s,
	}
}
