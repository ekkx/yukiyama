package yukiyama

import "sync"

// Session holds the authenticated user_id and token. All access is guarded by
// mu; copy via Snapshot for read access from hot paths.
type Session struct {
	mu     sync.RWMutex
	userID int32  // 0 when not authenticated
	token  string // "" when not authenticated
}

// Snapshot returns the current user_id, token, and ok=true when authenticated.
// When ok is false, callers should trigger Login (or surface a "not
// authenticated" error to the user).
func (s *Session) Snapshot() (userID int32, token string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userID, s.token, s.userID != 0 && s.token != ""
}

// Set stores the authenticated identity. Safe to call from any goroutine.
func (s *Session) Set(userID int32, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userID = userID
	s.token = token
}

// Clear wipes the cached identity. Called after error_code 103 detection so the
// next request triggers a fresh Login.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userID = 0
	s.token = ""
}

// IsAuthenticated reports whether Snapshot would return ok=true.
func (s *Session) IsAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userID != 0 && s.token != ""
}
