package logger

// Active-file size rotation (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.2).
//
// rotation.go cuts a new file when the wall-clock window turns over. That alone
// bounds nothing: the window is time, not bytes, so one busy day produces one
// unbounded file, and the directory size ceiling in disk_guard.go can only
// delete files that have ALREADY been rotated -- it explicitly reports that the
// active file alone exceeds the budget and that it cannot help.
//
// This file adds the missing half: the writer counts what it has written and
// cuts the file at LOG_MAX_ACTIVE_FILE_SIZE_MB. The rotated file keeps the
// window stamp plus a sequence suffix, so it collides with nothing and is still
// matched by both sweeps -- isLogFileName for the size and free-disk guards,
// parseRotationStamp for the writer's own age-based purge.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	errors "github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
)

// rotationReadDir is the directory reader used to discover a sequence after a
// process restart. Tests replace it to prove normal in-process rotations do
// not rescan the growing directory.
var rotationReadDir = os.ReadDir

const (
	// rotationSequenceDigits is the zero-padded width of the size-rotation
	// suffix appended to a time-window filename, as in
	// "oneapi-20260905-0001.log".
	//
	// The suffix exists so a size rotation cannot collide with the time-window
	// name it was cut from: without it the writer would have to reuse
	// "oneapi-20260905.log" and either truncate the previous file or refuse to
	// rotate at all (W0.2).
	rotationSequenceDigits = 4

	// maxRotationSequence bounds how many size rotations one time window may
	// produce. Reaching it means the ceiling is far too small for the write
	// rate; the writer reports that rather than spinning through filenames.
	maxRotationSequence = 9999
)

// activeFileCeilingBytes resolves the byte ceiling a rotation writer enforces on
// the file it currently holds open.
//
// It is deliberately resolved once, when the sink is built, so a single write
// path never reads mutable process configuration: the sink is created from
// SetupLogger after configuration has been loaded.
//
// ONLY_ONE_LOG_FILE disables it. That combination is contradictory -- a single
// file cannot also be bounded by rotating it -- and the logger refuses to
// pretend otherwise by silently truncating or by counting bytes it will never
// act on. Rejecting the combination at startup belongs to common/config; this
// package only guarantees the size rotation is inert and says so in
// SetupLogger.
//
// Parameters: none.
//
// Return values:
//   - int64: the ceiling in bytes, or zero when size rotation is disabled.
func activeFileCeilingBytes() int64 {
	if config.OnlyOneLogFile {
		return 0
	}
	return config.LogMaxActiveFileSizeBytes()
}

// ensureSizeHeadroom rotates the active file when appending n more bytes would
// take it past its ceiling.
//
// A single entry larger than the whole ceiling is written intact into an empty
// file rather than triggering an endless rotation loop: truncating a log line
// to satisfy a byte budget would corrupt the record, and the next write starts
// a fresh file anyway.
//
// The caller must hold w.mu.
//
// Parameters:
//   - n: the number of bytes about to be appended.
//
// Return values:
//   - error: wrapped failure from the size rotation.
func (w *rotationWriter) ensureSizeHeadroom(n int) error {
	if w.maxActiveBytes <= 0 || w.file == nil {
		return nil
	}
	if w.activeSize <= 0 {
		return nil
	}
	if w.activeSize+int64(n) <= w.maxActiveBytes {
		return nil
	}

	return errors.Wrap(w.rotateBySize(), "rotate log file by size")
}

// rotateBySize closes the active file, renames it to the next free sequenced
// sibling, and reopens an empty active file for the SAME time window.
//
// Renaming keeps the time-window name as the file a tailer follows, and the
// sequence suffix guarantees the rotated file cannot collide with the name the
// window rotation will produce. The rename happens while no descriptor is open
// so the behavior is identical on platforms that refuse to rename open files.
//
// Any failure still leaves a usable writer: the active file is reopened before
// the error is returned, so a naming problem degrades to "no size rotation"
// rather than to "no logging".
//
// The caller must hold w.mu.
//
// Parameters: none.
//
// Return values:
//   - error: wrapped failure from syncing, closing, naming, renaming, or
//     reopening the file.
func (w *rotationWriter) rotateBySize() error {
	if w.file == nil {
		return nil
	}

	if err := w.file.Sync(); err != nil {
		return errors.Wrap(err, "sync log file before size rotation")
	}
	if err := w.file.Close(); err != nil {
		return errors.Wrap(err, "close log file before size rotation")
	}
	w.file = nil
	previous := w.activePath
	w.activePath = ""
	w.activeSize = 0

	start, next := w.windowStart, w.nextCutover
	target, err := w.nextSequencePath(start)
	if err != nil {
		return errors.Wrap(w.reopenAfterFailedSizeRotation(start, next, err), "resolve rotated log file name")
	}

	if renameErr := os.Rename(previous, target); renameErr != nil && !os.IsNotExist(renameErr) {
		wrapped := errors.Wrapf(renameErr, "rename %s to %s", previous, target)
		return errors.Wrap(w.reopenAfterFailedSizeRotation(start, next, wrapped), "rotate log file by size")
	}

	if err := w.openNewFile(start, next, false); err != nil {
		return errors.Wrap(err, "open log file after size rotation")
	}

	return nil
}

// reopenAfterFailedSizeRotation restores a working active file after a size
// rotation could not complete.
//
// Parameters:
//   - start: the current window start.
//   - next: the current window cutover.
//   - cause: the failure that aborted the rotation.
//
// Return values:
//   - error: cause when the file was reopened, or a wrapped reopen failure,
//     which is the more severe of the two because logging would otherwise stop.
func (w *rotationWriter) reopenAfterFailedSizeRotation(start, next time.Time, cause error) error {
	if err := w.openNewFile(start, next, false); err != nil {
		return errors.Wrapf(err, "reopen log file after failed size rotation (cause: %v)", cause)
	}
	return cause
}

// nextSequencePath returns the next unused sequenced filename for a window.
//
// Parameters:
//   - start: the window the rotated file belongs to.
//
// Return values:
//   - string: the full path to use for the rotated file.
//   - error: wrapped stat failure, or an exhaustion error when the window has
//     already used every sequence slot.
func (w *rotationWriter) nextSequencePath(start time.Time) (string, error) {
	for seq := w.sequence + 1; seq <= maxRotationSequence; seq++ {
		candidate := w.sequencePathFor(start, seq)
		_, err := os.Stat(candidate)
		switch {
		case err == nil:
			continue
		case os.IsNotExist(err):
			w.sequence = seq
			return candidate, nil
		default:
			return "", errors.Wrapf(err, "stat candidate rotated log %s", candidate)
		}
	}

	return "", errors.Errorf("exhausted %d size rotation slots for window %s",
		maxRotationSequence, start.Format(time.RFC3339))
}

// sequencePathFor builds the filename of one size-rotated file.
//
// Parameters:
//   - start: the window the file belongs to.
//   - seq: the rotation sequence within that window.
//
// Return values:
//   - string: the full path, for example ".../oneapi-20260905-0001.log".
func (w *rotationWriter) sequencePathFor(start time.Time, seq int) string {
	stamp := start.Format(w.interval.filenameLayout())
	name := fmt.Sprintf("%s-%s-%0*d%s", w.loggerName, stamp, rotationSequenceDigits, seq, w.extension)
	return filepath.Join(w.baseDir, name)
}

// highestSequence reports the largest size-rotation suffix already present on
// disk for a window, so a restarted process resumes numbering instead of
// colliding with the files it wrote before.
//
// A directory that cannot be read yields zero rather than an error: the caller
// is opening a log file, and nextSequencePath re-checks every candidate with
// stat before using it.
//
// Parameters:
//   - start: the window to inspect.
//
// Return values:
//   - int: the highest sequence found, or zero when the window has none.
func (w *rotationWriter) highestSequence(start time.Time) int {
	entries, err := rotationReadDir(w.baseDir)
	if err != nil {
		return 0
	}

	prefix := w.loggerName + "-" + start.Format(w.interval.filenameLayout()) + "-"
	highest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || filepath.Ext(name) != w.extension {
			continue
		}
		seq, convErr := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, prefix), w.extension))
		if convErr != nil || seq <= highest {
			continue
		}
		highest = seq
	}

	return highest
}

// isAllDigits reports whether every rune in s is an ASCII digit.
//
// Parameters:
//   - s: the candidate sequence suffix.
//
// Return values:
//   - bool: true for a non-empty run of digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
