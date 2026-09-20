package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

const storeFileName = "policy.json"

// Entry is one enrolled Launcher's stored policy.
type Entry struct {
	IdentityKey string                 `json:"-"`
	Kind        contracts.LauncherKind `json:"kind"`
	Level       contracts.PolicyLevel  `json:"level"`
}

var storeMu sync.Mutex

// Path returns the enrollment store's file path, creating its parent
// directory if needed. It's deliberately under the config dir, not the
// state dir: enrollment is a user's deliberate, edit-worthy configuration
// (see docs/spec/cmdwarden.md#6), not an operational log like the audit
// trail.
func Path() (string, error) {
	dir, err := xdgpaths.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("policy: %w", err)
	}
	return filepath.Join(dir, storeFileName), nil
}

func load() (map[string]Entry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("policy: reading %s: %w", path, err)
	}
	var entries map[string]Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("policy: parsing %s: %w", path, err)
	}
	if entries == nil {
		entries = map[string]Entry{}
	}
	return entries, nil
}

func save(entries map[string]Entry) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("policy: marshaling store: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "policy-*.json.tmp")
	if err != nil {
		return fmt.Errorf("policy: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("policy: writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("policy: syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("policy: closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("policy: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("policy: replacing %s: %w", path, err)
	}
	return nil
}

// Enroll registers identityKey with the given kind, explicitly — enrollment
// is never automatic (see the package doc). Enrolling an already-enrolled
// key updates its kind and resets it to that kind's default level; use Set
// afterward to pick a non-default level.
func Enroll(identityKey string, kind contracts.LauncherKind) (Entry, error) {
	level, err := contracts.DefaultLevelForKind(kind)
	if err != nil {
		return Entry{}, err
	}

	storeMu.Lock()
	defer storeMu.Unlock()

	entries, err := load()
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{IdentityKey: identityKey, Kind: kind, Level: level}
	entries[identityKey] = entry
	if err := save(entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Unenroll removes identityKey from the store. Unenrolling a key that was
// never enrolled is not an error.
func Unenroll(identityKey string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	entries, err := load()
	if err != nil {
		return err
	}
	delete(entries, identityKey)
	return save(entries)
}

// Set changes an already-enrolled Launcher's level. It errors if
// identityKey isn't enrolled — there's no implicit enroll-on-set, matching
// "enrollment is explicit only."
func Set(identityKey string, level contracts.PolicyLevel) (Entry, error) {
	switch level {
	case contracts.LevelDeny, contracts.LevelRead, contracts.LevelTrusted, contracts.LevelFull:
	default:
		return Entry{}, fmt.Errorf("policy: unrecognized level %q", level)
	}

	storeMu.Lock()
	defer storeMu.Unlock()

	entries, err := load()
	if err != nil {
		return Entry{}, err
	}
	entry, ok := entries[identityKey]
	if !ok {
		return Entry{}, fmt.Errorf("policy: %q is not enrolled (run `cw policy enroll` first)", identityKey)
	}
	entry.IdentityKey = identityKey
	entry.Level = level
	entries[identityKey] = entry
	if err := save(entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// List returns every enrolled Launcher, sorted by identity key for stable
// output.
func List() ([]Entry, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	entries, err := load()
	if err != nil {
		return nil, err
	}
	list := make([]Entry, 0, len(entries))
	for key, e := range entries {
		e.IdentityKey = key
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].IdentityKey < list[j].IdentityKey })
	return list, nil
}

// Resolve returns identityKey's enrolled level, or (LevelDeny, false) if
// it's not enrolled — "an unenrolled Launcher's identity key defaults to
// Deny" is enforced here, at the one place every caller (the dry-run check,
// and later the real gate) goes through.
func Resolve(identityKey string) (level contracts.PolicyLevel, enrolled bool, err error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	entries, err := load()
	if err != nil {
		return "", false, err
	}
	entry, ok := entries[identityKey]
	if !ok {
		return contracts.LevelDeny, false, nil
	}
	return entry.Level, true, nil
}
