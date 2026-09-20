// Package audit is the NDJSON gate-decision log: one JSON object per line,
// one line per gate decision (auto-allow, allow-once, deny, unavailable,
// session-grant, session-allow), under CmdWarden-Omarchy's state directory.
// Every gate/vault code path that releases a secret or allows a gated
// command MUST call Log first and abort (fail closed) if it returns an
// error — see the package doc on Log.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

const logFileName = "gate-decisions.ndjson"

// retention window from the Windows CmdWarden audit spec ("~30 days, prune
// older").
const RetentionWindow = 30 * 24 * time.Hour

var writeMu sync.Mutex

// Path returns the audit log's file path, creating its parent directory if
// needed. It does not create the file itself — Log does that on first
// write.
func Path() (string, error) {
	dir, err := xdgpaths.StateDir()
	if err != nil {
		return "", fmt.Errorf("audit: %w", err)
	}
	return filepath.Join(dir, logFileName), nil
}

// Log appends one NDJSON row for a gate decision. It fsyncs before
// returning, and returns a non-nil error if the record is invalid or if the
// write/sync fails for any reason (disk full, permissions, ...).
//
// Callers MUST treat a non-nil error from Log as "fail closed": a secret
// must not be released and a command must not be allowed to proceed if its
// decision couldn't be durably recorded. Log never returns partial success.
func Log(rec contracts.AuditRecord) error {
	if err := validate(rec); err != nil {
		return fmt.Errorf("audit: refusing to log invalid record: %w", err)
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("audit: marshaling record: %w", err)
	}
	line = append(line, '\n')

	path, err := Path()
	if err != nil {
		return err
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: opening %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("audit: writing to %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("audit: syncing %s: %w", path, err)
	}
	return nil
}

func validate(rec contracts.AuditRecord) error {
	if rec.Decision == "" {
		return fmt.Errorf("decision is required")
	}
	switch rec.Decision {
	case contracts.DecisionAutoAllow, contracts.DecisionAllowOnce, contracts.DecisionDeny,
		contracts.DecisionUnavailable, contracts.DecisionSessionGrant, contracts.DecisionSessionAllow:
	default:
		return fmt.Errorf("unrecognized decision %q", rec.Decision)
	}
	if rec.Tool == "" {
		return fmt.Errorf("tool is required")
	}
	if rec.ReasonCode == "" {
		return fmt.Errorf("reason_code is required")
	}
	return nil
}

// ReadAll returns every record currently in the log, oldest first. A
// missing log file is treated as empty, not an error — nothing has been
// audited yet.
func ReadAll() ([]contracts.AuditRecord, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit: opening %s: %w", path, err)
	}
	defer f.Close()

	var records []contracts.AuditRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec contracts.AuditRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("audit: parsing %s: %w", path, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("audit: reading %s: %w", path, err)
	}
	return records, nil
}

// Follow writes newly appended records to w as they're logged, polling the
// file for growth, until ctx-equivalent stop is signalled via the returned
// stop channel semantics handled by the caller (cw audit --follow uses
// signal.NotifyContext upstream and just stops calling Follow's step
// function — see cliapp/audit.go). It blocks until stopped.
func Follow(w io.Writer, stop <-chan struct{}) error {
	path, err := Path()
	if err != nil {
		return err
	}

	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			f, err := os.Open(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("audit: opening %s: %w", path, err)
			}
			if _, err := f.Seek(offset, io.SeekStart); err == nil {
				if _, copyErr := io.Copy(w, f); copyErr == nil {
					if cur, statErr := f.Seek(0, io.SeekCurrent); statErr == nil {
						offset = cur
					}
				}
			}
			f.Close()
		}
	}
}
