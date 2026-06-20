package yukiyama

import (
	"testing"
)

// TestCheckinService_Wired is a construction smoke test: NewClient must
// initialize the Checkin service field, and the back-pointer must point at
// the same Client. Catches any future regression where someone adds a new
// service struct but forgets to wire it in NewClient.
func TestCheckinService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Checkin == nil {
		t.Fatal("client.Checkin is nil; NewClient did not initialize CheckinService")
	}
	if c.Checkin.c != c {
		t.Errorf("client.Checkin.c = %p, want %p (back-pointer mis-wired)", c.Checkin.c, c)
	}
}

// TestCheckinService_OptionsConstruction is a compile-time guard. Each
// Options struct is constructed with every field populated so that any
// rename or type change in the struct definitions surfaces at `go test`
// time rather than at a downstream caller.
func TestCheckinService_OptionsConstruction(t *testing.T) {
	var (
		s         = "x"
		i32 int32 = 1
		f32       = float32(1.5)
	)

	_ = CheckIsCheckedInOptions{}
	_ = GetCheckinTimelineOptions{
		Max:           &i32,
		Offset:        &i32,
		Order:         &s,
		Group:         &s,
		TargetUser:    &s,
		TargetSkiarea: &s,
		TargetTopics:  &s,
		FindOption:    &s,
		IsDebugger:    &i32,
		ApiVer:        &i32,
	}
	_ = GetCheckinTimelineWithTopicsOptions{
		Max:           &i32,
		Offset:        &i32,
		Order:         &s,
		Group:         &s,
		TargetUser:    &s,
		TargetSkiarea: &s,
		TargetTopics:  &s,
		FindOption:    &s,
		IsDebugger:    &i32,
		ApiVer:        &i32,
		Cmd:           &s,
	}
	_ = ListCheckinHistoryOptions{
		Max:        &i32,
		Offset:     &i32,
		SeasonFrom: &s,
		SeasonTo:   &s,
		IsDebugger: &i32,
	}
	_ = ListCheckinInterestUsersOptions{
		CheckinID: &i32,
		TopicsID:  &i32,
	}
	_ = StartCheckinOptions{
		IsOpen:           &i32,
		IsAutopost:       &i32,
		Weather:          &s,
		SnowState:        &s,
		CourceState:      &s,
		Temperature:      &s,
		SnowAccumulation: &s,
	}
	_ = EndCheckinOptions{
		MaxSpeed:      &f32,
		SlideDistance: &f32,
	}
	_ = PostOfflineCheckinOptions{
		OpenRange:      &f32,
		Title:          &s,
		TimelineLayout: &s,
		YoutubeUrl:     &s,
	}
	_ = EditCheckinGpslogOptions{
		SkiareaID: &i32,
		Start:     &s,
		Goals:     &s,
	}
	_ = JoinCheckinGroupOptions{
		Password: &s,
	}
	_ = ExitCheckinGroupOptions{
		Password: &s,
	}
}
