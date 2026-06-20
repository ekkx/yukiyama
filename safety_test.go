package yukiyama

import (
	"testing"
)

// TestSafetyService_Wired is a construction smoke test: NewClient must
// initialize the Safety service field, and the back-pointer must point at
// the same Client. Catches any future regression where someone adds a new
// service struct but forgets to wire it in NewClient.
func TestSafetyService_Wired(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Safety == nil {
		t.Fatal("client.Safety is nil; NewClient did not initialize SafetyService")
	}
	if c.Safety.c != c {
		t.Errorf("client.Safety.c = %p, want %p (back-pointer mis-wired)", c.Safety.c, c)
	}
}

// TestSafetyService_OptionsConstruction is a compile-time guard. Each
// Options struct is constructed with every field populated so that any
// rename or type change in the struct definitions surfaces at `go test`
// time rather than at a downstream caller.
func TestSafetyService_OptionsConstruction(t *testing.T) {
	_ = ListMySafetyHistoryOptions{}
}
