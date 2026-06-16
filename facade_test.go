package yukiyama

import "testing"

// TestFacade_OptionsConstruction is a compile-time guard. Each Options
// struct is constructed with every field populated so that any rename or
// type change in the struct definitions surfaces at `go test` time rather
// than at a downstream caller.
func TestFacade_OptionsConstruction(t *testing.T) {
	var (
		s         = "x"
		i32 int32 = 1
	)

	_ = ListDistributionNotificationsOptions{
		Max:    &i32,
		Offset: &i32,
	}

	_ = CheckinTimelineOptions{
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

	_ = ListSchedulesOptions{
		Type: &s,
		From: &s,
		To:   &s,
	}

	_ = ListVisitedSkiareasOptions{
		IsDebugger: &i32,
	}

	_ = ListVisitedSkiareasWithStatsOptions{
		SeasonFrom: &s,
		SeasonTo:   &s,
		SeasonYear: &i32,
		Mode:       &s,
		IsDebugger: &i32,
	}

	_ = CheckinTimelineWithTopicsOptions{
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
}
