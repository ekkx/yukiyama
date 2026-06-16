package yukiyama

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFileSessionStore_Roundtrip verifies that a Save() followed by Load()
// returns equivalent data (modulo SavedAt being auto-populated).
func TestFileSessionStore_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSessionStore(filepath.Join(dir, "session.json"))
	ctx := context.Background()

	in := &PersistedSession{UserID: 12345, Token: "deadbeefcafef00d"}
	if err := store.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil after Save")
	}
	if got.UserID != in.UserID {
		t.Errorf("UserID: got %d, want %d", got.UserID, in.UserID)
	}
	if got.Token != in.Token {
		t.Errorf("Token: got %q, want %q", got.Token, in.Token)
	}
	if got.SavedAt == "" {
		t.Error("SavedAt: expected auto-populated UTC timestamp, got empty string")
	}
}

// TestFileSessionStore_Clear verifies that Clear() removes the file and that
// a subsequent Load() returns (nil, nil) — the "no session" state.
func TestFileSessionStore_Clear(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSessionStore(filepath.Join(dir, "session.json"))
	ctx := context.Background()

	if err := store.Save(ctx, &PersistedSession{UserID: 1, Token: "tok"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load after Clear: %v", err)
	}
	if got != nil {
		t.Errorf("Load after Clear: got %+v, want nil", got)
	}
	// Clear is idempotent: a second call on a missing file must not error.
	if err := store.Clear(ctx); err != nil {
		t.Errorf("Clear (idempotent): %v", err)
	}
}

// TestFileSessionStore_DefaultPath verifies that an empty path falls back to a
// non-empty default. We don't pin the exact value because it depends on env
// (XDG_CONFIG_HOME vs HOME vs TempDir fallback) — we just assert that the
// constructor never produces an unusable store.
func TestFileSessionStore_DefaultPath(t *testing.T) {
	store := NewFileSessionStore("")
	if store.Path == "" {
		t.Fatal("default path should not be empty")
	}
	// Sanity-check that the path contains "yukiyama" so a typo in
	// defaultSessionPath() (e.g. missing dir name) would fail this test.
	if !contains(store.Path, "yukiyama") {
		t.Errorf("default path %q should contain 'yukiyama'", store.Path)
	}
}

// TestFileSessionStore_FilePermissions verifies the on-disk file is 0600.
// Skipped on Windows where chmod semantics differ.
func TestFileSessionStore_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	store := NewFileSessionStore(path)

	if err := store.Save(context.Background(), &PersistedSession{UserID: 1, Token: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode: got %#o, want 0600", perm)
	}
}

// TestFileSessionStore_LoadNonexistent verifies the contract that an absent
// file is the "no session" state, not an error.
func TestFileSessionStore_LoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSessionStore(filepath.Join(dir, "nope.json"))
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load nonexistent: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Load nonexistent: got %+v, want nil", got)
	}
}

// TestFileSessionStore_LoadEmpty covers the partial-write edge case where the
// file exists but has zero/blank fields. Treating it as "no session" matches
// the Load contract.
func TestFileSessionStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	store := NewFileSessionStore(path)
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if got != nil {
		t.Errorf("Load empty: got %+v, want nil", got)
	}
}

// TestFileSessionStore_WireShape locks the on-disk JSON keys so external
// consumers (other SDKs, audit tools, hand-edits) can rely on the schema.
func TestFileSessionStore_WireShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	store := NewFileSessionStore(path)
	if err := store.Save(context.Background(), &PersistedSession{UserID: 42, Token: "t"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	for _, key := range []string{"user_id", "token", "saved_at"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in serialized form: %s", key, string(raw))
		}
	}
}

// TestNoopSessionStore covers the default store's no-op contract.
func TestNoopSessionStore(t *testing.T) {
	store := NoopSessionStore{}
	ctx := context.Background()

	got, err := store.Load(ctx)
	if err != nil {
		t.Errorf("Load: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Load: got %+v, want nil", got)
	}
	if err := store.Save(ctx, &PersistedSession{UserID: 1, Token: "x"}); err != nil {
		t.Errorf("Save: %v", err)
	}
	if err := store.Clear(ctx); err != nil {
		t.Errorf("Clear: %v", err)
	}
}

// contains is a tiny strings.Contains replica that keeps this file free of
// imports outside the standard library subset we already need.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
