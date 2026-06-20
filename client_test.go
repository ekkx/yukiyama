package yukiyama

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// recordingStore is an in-memory SessionStore that tracks call counts and
// retains the last saved value. It is used to assert the wire-up between
// session-lifecycle events (NewClient, User.Login, User.Logout) and the store.
type recordingStore struct {
	mu     sync.Mutex
	loads  int
	saves  int
	clears int
	state  *PersistedSession
}

func (r *recordingStore) Load(context.Context) (*PersistedSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	if r.state == nil {
		return nil, nil
	}
	cp := *r.state
	return &cp, nil
}

func (r *recordingStore) Save(_ context.Context, s *PersistedSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves++
	if s == nil {
		r.state = nil
		return nil
	}
	cp := *s
	r.state = &cp
	return nil
}

func (r *recordingStore) Clear(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clears++
	r.state = nil
	return nil
}

// TestClient_AutoLoadSession_OnStartup verifies that NewClient with a
// SessionStore containing a non-empty session seeds the in-memory Session.
func TestClient_AutoLoadSession_OnStartup(t *testing.T) {
	store := &recordingStore{
		state: &PersistedSession{UserID: 12345, Token: "stored-token"},
	}
	client, err := NewClient(
		WithSessionStore(store),
		// Disable autoLogin so we don't accidentally fire a Login round-trip
		// if the store fails to hydrate — that would make a failure look
		// like a network bug instead of a wiring bug.
		WithAutoLogin(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if !client.IsAuthenticated() {
		t.Fatal("expected client to be authenticated from store hydration")
	}
	if got := client.CurrentUserID(); got != 12345 {
		t.Errorf("CurrentUserID: got %d, want 12345", got)
	}
	if got := client.CurrentToken(); got != "stored-token" {
		t.Errorf("CurrentToken: got %q, want stored-token", got)
	}
	store.mu.Lock()
	loads := store.loads
	store.mu.Unlock()
	if loads != 1 {
		t.Errorf("expected Load to be called exactly once during NewClient, got %d", loads)
	}
}

// TestClient_AutoLoadSession_Disabled verifies that WithAutoLoadSession(false)
// suppresses the startup Load() call.
func TestClient_AutoLoadSession_Disabled(t *testing.T) {
	store := &recordingStore{
		state: &PersistedSession{UserID: 1, Token: "x"},
	}
	client, err := NewClient(
		WithSessionStore(store),
		WithAutoLoadSession(false),
		WithAutoLogin(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.IsAuthenticated() {
		t.Error("expected unauthenticated client when autoLoadSession=false")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.loads != 0 {
		t.Errorf("expected zero Load calls when autoLoadSession=false, got %d", store.loads)
	}
}

// TestClient_AutoLoadSession_NoFile verifies graceful handling of an empty
// store: NewClient succeeds but the session stays empty.
func TestClient_AutoLoadSession_NoFile(t *testing.T) {
	store := &recordingStore{} // no state
	client, err := NewClient(
		WithSessionStore(store),
		WithAutoLogin(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.IsAuthenticated() {
		t.Error("expected unauthenticated client when store has no state")
	}
}

// TestClient_Login_PersistsToStore verifies the Login → Save wire-up. We use
// a FileSessionStore + a temp dir so the test covers the real on-disk path.
// We can't drive a real Login round-trip without network, so we inject the
// session via SetSession and call store.Save manually — but that would only
// test the store, not the wiring. Instead, drive Login through a fake
// transport that returns a canned login response.
func TestClient_Login_PersistsToStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	// Canned response matching what /user/login returns on success. The
	// nested LoginProfile has its own required-field validator (id,
	// userName, objectId, mailAddress, created_at, updated_at); supply
	// minimally-valid values so the decode side succeeds.
	body := `{"status":"1","id":"42","token":"new-token","is_debugger":"0","error":"",` +
		`"profile":{"id":"42","userName":"u","objectId":"o","mailAddress":"m@example.com",` +
		`"created_at":"2020-01-01 00:00:00","updated_at":"2020-01-01 00:00:00"}}`
	fake := &fakeRoundTripper{responses: []*http.Response{
		newJSONResponse(200, body),
	}}

	client, err := NewClient(
		WithCredentials("user@example.com", "password"),
		WithSessionStore(NewFileSessionStore(path)),
		WithAutoLogin(false),
		WithHTTPClient(&http.Client{Transport: fake}),
		// Disable autoLoad so the initial state is "no session" and Login
		// has something to do.
		WithAutoLoadSession(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.User.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got := client.CurrentUserID(); got != 42 {
		t.Errorf("CurrentUserID: got %d, want 42", got)
	}

	// Confirm the persistent store now has the new session.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var p PersistedSession
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.UserID != 42 {
		t.Errorf("persisted UserID: got %d, want 42", p.UserID)
	}
	if p.Token != "new-token" {
		t.Errorf("persisted Token: got %q, want new-token", p.Token)
	}
}

// TestClient_Logout_ClearsStore verifies the Logout → Clear wire-up.
func TestClient_Logout_ClearsStore(t *testing.T) {
	store := &recordingStore{
		state: &PersistedSession{UserID: 7, Token: "tok"},
	}
	client, err := NewClient(
		WithSessionStore(store),
		WithAutoLogin(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if !client.IsAuthenticated() {
		t.Fatal("expected hydrated session")
	}

	client.User.Logout()

	if client.IsAuthenticated() {
		t.Error("expected session cleared after Logout")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.clears != 1 {
		t.Errorf("expected exactly one Clear call, got %d", store.clears)
	}
	if store.state != nil {
		t.Errorf("expected store state nil after Logout, got %+v", store.state)
	}
}

// TestClient_NilSessionStore_DefaultsToNoop guards against a regression where
// WithSessionStore(nil) would NPE inside Logout/Login. The contract is that
// nil falls back to NoopSessionStore.
func TestClient_NilSessionStore_DefaultsToNoop(t *testing.T) {
	client, err := NewClient(
		WithSessionStore(nil),
		WithAutoLogin(false),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Should not panic.
	client.User.Logout()
	if _, ok := client.SessionStore().(NoopSessionStore); !ok {
		t.Errorf("expected NoopSessionStore fallback, got %T", client.SessionStore())
	}
}
