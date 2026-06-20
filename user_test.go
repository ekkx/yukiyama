package yukiyama

import (
	"testing"

	gen "github.com/ekkx/yukiyama/gen"
)

// TestUserService_Wired is a construction smoke test: NewClient must
// initialize the User service field, and the back-pointer must point at
// the same Client.
func TestUserService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.User == nil {
		t.Fatal("client.User is nil; NewClient did not initialize UserService")
	}
	if c.User.c != c {
		t.Errorf("client.User.c = %p, want %p (back-pointer mis-wired)", c.User.c, c)
	}
}

// TestUserService_OptionsConstruction is a compile-time guard. Every Options
// struct is constructed with every field populated so that any rename or
// type change in the struct definitions surfaces at `go test` time rather
// than at a downstream caller.
func TestUserService_OptionsConstruction(t *testing.T) {
	var (
		s           = "x"
		i32 int32   = 1
		f32 float32 = 1
		adt         = gen.ADACCESSTYPE_VIEW
	)

	_ = GetMyPageOptions{IsDebugger: &i32}
	_ = SearchUsersOptions{FindOption: &s, Max: &i32, Offset: &i32}
	_ = ListRecommendedUsersOptions{}
	_ = ListUserFollowersOptions{Max: &i32, Offset: &i32, Keyword: &s}
	_ = ListUserFollowingOptions{Max: &i32, Offset: &i32, Keyword: &s}
	_ = ListUserSchedulesOptions{Type: &s, From: &s, To: &s}
	_ = ListVisitedSkiareasOptions{IsDebugger: &i32}
	_ = ListVisitedSkiareasWithStatsOptions{
		SeasonFrom: &s,
		SeasonTo:   &s,
		SeasonYear: &i32,
		Mode:       &s,
		IsDebugger: &i32,
	}
	_ = ListRideoutsOptions{SeasonFrom: &s, SeasonTo: &s, SeasonYear: &s}
	_ = GetTotalRideoutOptions{DebugUserID: &s}
	_ = GetRidingGraphOptions{
		SelectTerm:   &s,
		SelectTarget: &s,
		PreTarget:    &s,
		DebugUserID:  &s,
	}
	_ = ListCheckinSeasonsOptions{IsDebugger: &i32}
	_ = ListGlidedTogetherUsersOptions{SeasonFrom: &s, SeasonTo: &s, SeasonYear: &s}
	_ = ListFavoriteSkiareasOptions{SkiareaID: &i32}
	_ = ListMyCouponsOptions{}
	_ = ListMyUserGroupsOptions{}
	_ = ListBadgesOptions{}
	_ = ListNewBadgesOptions{}
	_ = UpdateBadgeSettingOptions{
		Type:         &f32,
		Rank:         &f32,
		Index:        &f32,
		IsDelete:     &i32,
		EventBadgeID: &i32,
	}
	_ = ListAllStancesOptions{}
	_ = ListStancesNewOptions{Max: &i32, Offset: &i32}
	_ = UpdateFavoriteStanceCustomOptions{
		CustomL: &f32,
		CustomR: &f32,
		CustomW: &f32,
	}
	_ = ListBlockedUsersOptions{}
	_ = ListGeofencingTargetsOptions{}
	_ = JoinScheduleOptions{IsPushCheckinAlert: &i32}
	_ = EditUserOptions{
		Nickname:     &s,
		Appeal:       &s,
		InstagramURL: &s,
		FacebookURL:  &s,
		TwitterURL:   &s,
		YoutubeURL:   &s,
		TiktokURL:    &s,
	}
	_ = EditProfileOptions{
		MainGear:             &s,
		Birthday:             &s,
		Gender:               &f32,
		Birthplace:           &s,
		Style:                &s,
		Level:                &s,
		Experience:           &s,
		HomeSkiareaID:        &i32,
		BirthmonthdayVisible: &i32,
		BirthyearVisible:     &i32,
		BirthplaceVisible:    &i32,
		IsShowSupporterLabel: &i32,
	}
	_ = EditUserNewOptions{
		Nickname:             &s,
		Appeal:               &s,
		InstagramURL:         &s,
		FacebookURL:          &s,
		TwitterURL:           &s,
		YoutubeURL:           &s,
		TiktokURL:            &s,
		MainGear:             &s,
		Birthday:             &s,
		Gender:               &f32,
		Birthplace:           &s,
		Style:                &s,
		Level:                &s,
		Experience:           &s,
		HomeSkiareaID:        &i32,
		BirthmonthdayVisible: &i32,
		BirthyearVisible:     &i32,
		BirthplaceVisible:    &i32,
		IsShowSupporterLabel: &i32,
	}
	_ = EditUserGearOptions{
		GearBoard:   &s,
		GearSki:     &s,
		GearBinding: &s,
		GearBoots:   &s,
		GearGoggle:  &s,
		GearGloves:  &s,
		GearWear:    &s,
		GearPants:   &s,
		GearPole:    &s,
		GearHead:    &s,
		GearOther:   &s,
	}
	_ = UpdatePushSettingOptions{
		IsPushInterest:       &i32,
		IsPushComment:        &i32,
		IsPushFollow:         &i32,
		IsPushScheduleCreate: &i32,
		IsPushScheduleJoin:   &i32,
		IsPushInfo:           &i32,
		IsPushCheckinAlert:   &i32,
	}
	_ = UpdateSupporterStatusOptions{
		IsSupporter:           &i32,
		PurchaseDate:          &s,
		PlanType:              &f32,
		ExpirationDate:        &s,
		Receipt:               &s,
		DeviceType:            &i32,
		TransactionID:         &s,
		OriginalTransactionID: &s,
	}
	_ = TrackAdAccessOptions{
		Type:     &adt,
		ShopAdID: &i32,
		StayTime: &s,
	}
}
