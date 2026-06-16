package yukiyama

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SessionStore is a pluggable persistence interface for the cached session.
//
// Implementations are expected to be safe for concurrent use. The SDK calls
// Load() at most once during NewClient (when WithAutoLoadSession is enabled,
// which is the default) and Save() after each successful Login (including the
// transparent re-login triggered by error_code 103). Clear() is called from
// Logout().
//
// Returning a nil PersistedSession with a nil error from Load indicates "no
// saved session" — this is the normal first-run state and is not an error.
//
// Save / Load failures are non-fatal: the SDK logs a warning via the configured
// Logger and continues. The motivation for this design is that a missing or
// transiently broken store (e.g. a $HOME with no write permission) should not
// prevent the caller from authenticating via the regular Login path.
//
// ctx is provided so implementations that talk to remote stores, keychains, or
// any other blocking backend can honor caller cancellation and deadlines.
type SessionStore interface {
	// Load reads the persisted session. Returns (nil, nil) when no session is
	// stored. Returns a non-nil error only on unrecoverable read failures.
	Load(ctx context.Context) (*PersistedSession, error)
	// Save persists the supplied session, replacing any prior value.
	Save(ctx context.Context, sess *PersistedSession) error
	// Clear removes any persisted session. A no-op when nothing is stored.
	Clear(ctx context.Context) error
}

// PersistedSession is the wire shape used to serialize sessions across process
// restarts. It is intentionally distinct from the in-memory Session type
// (which carries a sync.RWMutex and other private state) so that JSON
// encoders / decoders never touch the mutex.
//
// SavedAt is "YYYY-MM-DD HH:MM:SS" in UTC, populated by NewClient/Login. It is
// informational only; the SDK does not use it to invalidate the session.
type PersistedSession struct {
	UserID  int32  `json:"user_id"`
	Token   string `json:"token"`
	SavedAt string `json:"saved_at,omitempty"`
}

// NoopSessionStore is the default SessionStore. Load returns (nil, nil), Save
// and Clear succeed without doing anything. Useful when callers do not need
// cross-process persistence.
type NoopSessionStore struct{}

// Load implements SessionStore.
func (NoopSessionStore) Load(context.Context) (*PersistedSession, error) { return nil, nil }

// Save implements SessionStore.
func (NoopSessionStore) Save(context.Context, *PersistedSession) error { return nil }

// Clear implements SessionStore.
func (NoopSessionStore) Clear(context.Context) error { return nil }

// FileSessionStore persists the session as a JSON file at Path.
//
// Concurrency: writes are atomic via the temp-file + rename pattern. Reads
// are a single os.ReadFile call. There is no in-process mutex because each
// Save call writes the entire file in one go and the OS rename is atomic on
// the same filesystem.
//
// File mode: 0600 on the destination file. Parent directories are created
// with mode 0700 (Save) so any leaked credentials are limited to the owner.
type FileSessionStore struct {
	Path string
}

// NewFileSessionStore returns a FileSessionStore. When path is "" it falls back
// to $XDG_CONFIG_HOME/yukiyama/session.json, then $HOME/.config/yukiyama/session.json.
// As a last resort it falls back to a temp-dir path so the returned store is
// always usable (Save may still fail at write time if the chosen dir is
// unwritable, but construction never errors).
func NewFileSessionStore(path string) *FileSessionStore {
	if path == "" {
		path = defaultSessionPath()
	}
	return &FileSessionStore{Path: path}
}

// defaultSessionPath computes the default on-disk location for the session.
// The XDG spec is preferred; HOME-based fallback matches the v0 of the spec
// (~/.config). On systems where neither variable is set we degrade to the OS
// temp dir so the caller still gets a usable path.
func defaultSessionPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "yukiyama", "session.json")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "yukiyama", "session.json")
	}
	return filepath.Join(os.TempDir(), "yukiyama-session.json")
}

// Load implements SessionStore. Returns (nil, nil) when the file does not
// exist; that is the expected first-run state.
func (s *FileSessionStore) Load(_ context.Context) (*PersistedSession, error) {
	if s == nil || s.Path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var p PersistedSession
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	// A file with zeroed fields is equivalent to "no session". This guards
	// against a partial write where the JSON is structurally valid but the
	// fields are empty (e.g. a manual `{}` written by a curious user).
	if p.UserID == 0 || p.Token == "" {
		return nil, nil
	}
	return &p, nil
}

// Save implements SessionStore. Writes atomically via a sibling temp file +
// rename, and chmods the destination to 0600. SavedAt is auto-populated when
// the caller did not set it.
func (s *FileSessionStore) Save(_ context.Context, sess *PersistedSession) error {
	if s == nil || s.Path == "" {
		return errors.New("yukiyama: FileSessionStore.Path is empty")
	}
	if sess == nil {
		return errors.New("yukiyama: refusing to save nil session (use Clear instead)")
	}
	out := *sess
	if out.SavedAt == "" {
		out.SavedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	// Write to a sibling temp file then rename: this guarantees a partial
	// write never corrupts the destination, and the destination ends up with
	// the desired 0600 permission bit even on platforms where umask would
	// otherwise mask it (we chmod explicitly after creating).
	tmp, err := os.CreateTemp(dir, "session-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails before the rename.
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return err
	}
	// rename preserves the temp file's mode on POSIX, but be explicit in case
	// the temp dir's umask narrowed it (some hardened systems do this).
	if err := os.Chmod(s.Path, 0o600); err != nil {
		return err
	}
	return nil
}

// Clear implements SessionStore. Deleting a nonexistent file is not an error.
func (s *FileSessionStore) Clear(_ context.Context) error {
	if s == nil || s.Path == "" {
		return nil
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
