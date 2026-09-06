package logger

// Log-directory disk guards (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.3).
//
// Day-based retention alone cannot bound disk: at a few thousand requests per
// second a single day of logs can exceed the volume, and the sweeper would
// still consider every file "fresh". Two ceilings close that gap — a total size
// budget for the directory, and a free-space floor on the filesystem.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// levelEscalated records whether the free-disk guard has already raised the
// process log level, so recovery restores it exactly once.
var levelEscalated atomic.Bool

// logFile is one candidate for deletion, ordered oldest first.
type logFile struct {
	path    string
	size    int64
	modTime time.Time
}

// listLogFiles returns the log files in a directory, oldest first.
//
// Parameters:
//   - lg: the worker's logger; the package-level Logger must not be read from
//     the worker goroutine, which would race SetupEnhancedLogger.
//   - logDir: the directory to scan.
//
// Return values:
//   - []logFile: log files sorted by modification time, oldest first.
//   - int64: their total size in bytes.
//   - error: wrapped failure when the directory cannot be listed. A missing
//     directory is not an error; it yields an empty result.
func listLogFiles(lg glog.Logger, logDir string) ([]logFile, int64, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, errors.Wrap(err, "read log directory")
	}

	files := make([]logFile, 0, len(entries))
	var total int64

	for _, entry := range entries {
		if entry.IsDir() || !isLogFileName(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			lg.Warn("skip log file without metadata",
				zap.String("log_path", filepath.Join(logDir, entry.Name())),
				zap.Error(infoErr))
			continue
		}
		files = append(files, logFile{
			path:    filepath.Join(logDir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime().UTC(),
		})
		total += info.Size()
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	return files, total, nil
}

// logFileNamePrefix scopes deletion to files this process produced.
//
// The sink is built from filepath.Join(LogDir, "oneapi.log") and the rotation
// sink stamps its window into names like "oneapi-20260905.log", so every file
// this process owns starts with it.
const logFileNamePrefix = "oneapi"

// isLogFileName reports whether a directory entry is a log file THIS PROCESS
// produced.
//
// The prefix test is a safety requirement, not a tidiness one: an operator may
// point --log-dir at a shared directory such as /var/log or a bind mount also
// used by a sidecar. Matching any "*.log" there would delete another program's
// data, which nothing in this project is entitled to do.
//
// Parameters:
//   - name: the base file name.
//
// Return values:
//   - bool: true for this process's ".log" files and rotated ".log.*" variants.
func isLogFileName(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, logFileNamePrefix) {
		return false
	}
	return strings.HasSuffix(lower, ".log") || strings.Contains(lower, ".log.")
}

// enforceSizeCeiling deletes oldest-first until the directory fits its budget.
//
// The file the logger currently holds open is never deleted: unlinking it frees
// nothing until the next rotation, because the open descriptor keeps the inode
// alive, and the process would go on writing to a file no longer reachable by
// name. activeLogFile identifies it by name rather than by modification time,
// which a foreign or restored file could otherwise win.
//
// Parameters:
//   - lg: the worker's logger.
//   - logDir: the directory to bound.
//   - maxTotalBytes: the budget; values <= 0 disable the ceiling.
//
// Return values:
//   - int: how many files were deleted.
//   - error: wrapped failure when the directory cannot be listed.
func enforceSizeCeiling(lg glog.Logger, logDir string, maxTotalBytes int64) (int, error) {
	if maxTotalBytes <= 0 {
		return 0, nil
	}

	files, total, err := listLogFiles(lg, logDir)
	if err != nil {
		return 0, err
	}
	if total <= maxTotalBytes {
		return 0, nil
	}

	active := activeLogFile(logDir)

	deleted := 0
	for i := 0; i < len(files) && total > maxTotalBytes; i++ {
		if files[i].path == active {
			continue
		}
		if removeLogFile(lg, files[i]) {
			total -= files[i].size
			deleted++
		}
	}

	if total > maxTotalBytes {
		lg.Warn("log directory still exceeds its size ceiling after deleting rotated files",
			zap.String("log_dir", logDir),
			zap.Int64("total_bytes", total),
			zap.Int64("max_total_bytes", maxTotalBytes),
			zap.String("hint", "the active log file alone exceeds the budget; lower LOG_ROTATION_INTERVAL or raise LOG_MAX_TOTAL_SIZE_MB"))
	}

	return deleted, nil
}

// enforceFreeDiskFloor keeps free space above a floor, escalating the log level
// as a last resort.
//
// A gateway that dies because its disk filled is worse than a gateway that
// stops writing INFO, so when deleting every rotated log is not enough the
// process stops emitting anything below WARN until space recovers.
//
// Parameters:
//   - lg: the worker's logger.
//   - logDir: the directory whose filesystem is inspected.
//   - minFreeBytes: the floor; values <= 0 disable the guard.
//
// Return values:
//   - int: how many files were deleted.
//   - error: wrapped failure when free space cannot be read.
func enforceFreeDiskFloor(lg glog.Logger, logDir string, minFreeBytes int64) (int, error) {
	if minFreeBytes <= 0 || !diskFreeSupported() {
		return 0, nil
	}

	free, err := freeDiskBytes(logDir)
	if err != nil {
		return 0, err
	}
	if int64(free) >= minFreeBytes {
		restoreLogLevel(lg)
		return 0, nil
	}

	lg.Warn("free disk space below the configured floor, purging rotated logs",
		zap.String("log_dir", logDir),
		zap.Uint64("free_bytes", free),
		zap.Int64("min_free_bytes", minFreeBytes))

	files, _, err := listLogFiles(lg, logDir)
	if err != nil {
		return 0, err
	}

	active := activeLogFile(logDir)

	deleted := 0
	for i := 0; i < len(files); i++ {
		if files[i].path == active {
			continue
		}
		if !removeLogFile(lg, files[i]) {
			continue
		}
		deleted++
		if free, err = freeDiskBytes(logDir); err != nil {
			return deleted, err
		}
		if int64(free) >= minFreeBytes {
			restoreLogLevel(lg)
			return deleted, nil
		}
	}

	escalateLogLevel(lg, logDir, free, minFreeBytes)
	return deleted, nil
}

// activeLogFile returns the path of the newest file this process owns, which is
// the one the logger is writing to.
//
// Parameters:
//   - logDir: the directory being swept.
//
// Return values:
//   - string: the active file path, or "" when the directory holds none.
func activeLogFile(logDir string) string {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return ""
	}

	newest := ""
	var newestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !isLogFileName(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = filepath.Join(logDir, entry.Name())
			newestTime = info.ModTime()
		}
	}
	return newest
}

// removeLogFile deletes one log file, reporting whether it succeeded.
//
// Parameters:
//   - lg: the worker's logger.
//   - f: the file to remove.
//
// Return values:
//   - bool: true when the file is gone.
func removeLogFile(lg glog.Logger, f logFile) bool {
	if err := os.Remove(f.path); err != nil {
		if os.IsNotExist(err) {
			return true
		}
		lg.Warn("failed to delete log file", zap.String("log_path", f.path), zap.Error(err))
		return false
	}
	lg.Info("deleted log file to reclaim disk",
		zap.String("log_path", f.path),
		zap.Int64("size_bytes", f.size),
		zap.Time("modified_at", f.modTime))
	return true
}

// escalateLogLevel raises the process log level to warn so logging stops
// consuming the little disk that is left.
//
// Parameters:
//   - lg: the worker's logger. glog derives loggers with a SHARED atomic level,
//     so changing it here changes the level of the whole process.
//   - logDir: the directory being guarded, for the report.
//   - free: currently free bytes.
//   - minFreeBytes: the floor that was not met.
//
// Return values: none.
func escalateLogLevel(lg glog.Logger, logDir string, free uint64, minFreeBytes int64) {
	if levelEscalated.Swap(true) {
		return
	}
	lg.Error("free disk space still below the floor after purging rotated logs; "+
		"raising log level to warn until space recovers",
		zap.String("log_dir", logDir),
		zap.Uint64("free_bytes", free),
		zap.Int64("min_free_bytes", minFreeBytes))
	if err := lg.ChangeLevel(glog.LevelWarn); err != nil {
		lg.Warn("failed to raise log level under disk pressure", zap.Error(err))
	}
}

// restoreLogLevel returns the process log level to its configured value once
// free space recovers.
//
// Parameters:
//   - lg: the worker's logger, whose level is shared with the process logger.
//
// Return values: none.
func restoreLogLevel(lg glog.Logger) {
	if !levelEscalated.Swap(false) {
		return
	}
	level := defaultLevel()
	if err := lg.ChangeLevel(level); err != nil {
		lg.Warn("failed to restore log level after disk pressure eased", zap.Error(err))
		return
	}
	lg.Info("free disk space recovered, restored log level", zap.String("level", level.String()))
}
