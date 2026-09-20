package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Prune rewrites the audit log keeping only records newer than
// time.Now().Add(-maxAge), per the spec's ~30-day retention window
// (RetentionWindow). It writes to a temp file and renames it into place, so
// a crash mid-prune never leaves a truncated log.
func Prune(maxAge time.Duration) (kept, dropped int, err error) {
	records, err := ReadAll()
	if err != nil {
		return 0, 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	path, err := Path()
	if err != nil {
		return 0, 0, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "gate-decisions-*.ndjson.tmp")
	if err != nil {
		return 0, 0, fmt.Errorf("audit: creating temp file for prune: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	writeMu.Lock()
	defer writeMu.Unlock()

	for _, rec := range records {
		if rec.Timestamp.Before(cutoff) {
			dropped++
			continue
		}
		kept++
		line, err := json.Marshal(rec)
		if err != nil {
			tmp.Close()
			return 0, 0, fmt.Errorf("audit: re-marshaling record during prune: %w", err)
		}
		if _, err := tmp.Write(append(line, '\n')); err != nil {
			tmp.Close()
			return 0, 0, fmt.Errorf("audit: writing temp file during prune: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return 0, 0, fmt.Errorf("audit: syncing temp file during prune: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, 0, fmt.Errorf("audit: closing temp file during prune: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return 0, 0, fmt.Errorf("audit: chmod temp file during prune: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return 0, 0, fmt.Errorf("audit: replacing %s during prune: %w", path, err)
	}
	return kept, dropped, nil
}
