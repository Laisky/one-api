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
	"sync"
	"sync/atomic"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// levelEscalated records whether the free-disk guard has already raised the
// process log level, so recovery restores it exactly once.
var levelEscalated atomic.Bool

var activeLogFiles = struct {
	sync.RWMutex
	paths map[string]string
}{paths: make(map[string]string)}

// setActiveLogFile records the exact pathname held open by this process for a
// log directory. Directory metadata cannot identify this safely after a size
// rotation, because the active and renamed sibling may share an mtime.
//
// Parameters:
//   - path: the pathname held open by the writer.
//
// Return values: none.
func setActiveLogFile(path string) {
	if path == "" {
		return
	}
	cleaned := filepath.Clean(path)
	activeLogFiles.Lock()
	activeLogFiles.paths[filepath.Dir(cleaned)] = cleaned
	activeLogFiles.Unlock()
}

// clearActiveLogFile removes an ownership record only when it still names path.
//
// Parameters:
//   - path: the pathname a closing writer previously held open.
//
// Return values: none.
func clearActiveLogFile(path string) {
	if path == "" {
		return
	}
	cleaned := filepath.Clean(path)
	dir := filepath.Dir(cleaned)
	activeLogFiles.Lock()
	if activeLogFiles.paths[dir] == cleaned {
		delete(activeLogFiles.paths, dir)
	}
	activeLogFiles.Unlock()
}

// freeDiskProbe reads available space on the filesystem holding a path.
//
// It is a variable so hysteresis can be tested at all: the entry floor, the
// recovery floor and the band between them are only distinguishable by driving
// free space to specific values, which no test can do to a real filesystem.
var freeDiskProbe = freeDiskBytes

// pressureReportInterval rate-limits the guard's own reporting.
//
// The pressure loop now samples every LOG_DISK_CHECK_INTERVAL_SEC (five seconds
// by default) instead of once per retention sweep, so a warning emitted on
// every observation would be roughly seventeen thousand times more log volume
// per day -- produced by the guard whose job is to produce less of it.
const pressureReportInterval = time.Minute

// pressureReport rate-limits one repeated guard message.
type pressureReport struct {
	mu     sync.Mutex
	lastAt time.Time
}

// allow reports whether the message may be emitted now, and records the
// emission when it may.
//
// Parameters:
//   - now: the current time, in UTC.
//
// Return values:
//   - bool: true when the caller should log.
func (r *pressureReport) allow(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.lastAt.IsZero() && now.Sub(r.lastAt) < pressureReportInterval {
		return false
	}
	r.lastAt = now
	return true
}

// reset clears the rate limiter so the next observation reports immediately.
//
// Parameters: none.
//
// Return values: none.
func (r *pressureReport) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAt = time.Time{}
}

var (
	// belowFloorReport rate-limits the "free disk below the floor" warning.
	belowFloorReport = &pressureReport{}
	// activeFileReport rate-limits the "active file over its ceiling" warning.
	activeFileReport = &pressureReport{}
)

// diskRecoveryFloor resolves the free-space level at which the guard leaves
// emergency mode.
//
// It clamps the configured hysteresis result to at least the entry floor. The
// margin is applied by multiplication in common/config, and a floor large
// enough to overflow int64 when multiplied would otherwise produce a recovery
// threshold BELOW the entry floor -- which would turn hysteresis into
// permanent, instantaneous recovery. Tests deliberately use such a floor to
// force the guard to exhaust its options.
//
// Parameters:
//   - minFreeBytes: the configured free-disk floor, in bytes.
//
// Return values:
//   - int64: the recovery threshold, never below minFreeBytes.
func diskRecoveryFloor(minFreeBytes int64) int64 {
	recovery := config.LogDiskRecoveryFloorBytes(minFreeBytes)
	if recovery < minFreeBytes {
		return minFreeBytes
	}
	return recovery
}

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
// The budget covers the ACTIVE file as well as the rotated ones. That matters
// because the active file is the only one the sweep cannot delete: unlinking it
// frees nothing until the next rotation, since the open descriptor keeps the
// inode alive, and the process would go on writing to a file no longer
// reachable by name. Excluding it from the total would therefore report the
// directory as compliant while the one file nothing can reclaim grows past the
// budget on its own -- which is precisely the hole W0.2 closes by rotating that
// file on size.
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
	var activeBytes int64
	for i := range files {
		if files[i].path == active {
			activeBytes = files[i].size
			break
		}
	}

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
			zap.Int64("active_file_bytes", activeBytes),
			zap.Int64("max_total_bytes", maxTotalBytes),
			zap.String("hint", "the active log file alone exceeds the budget; set LOG_MAX_ACTIVE_FILE_SIZE_MB so it is rotated by size, "+
				"lower LOG_ROTATION_INTERVAL, or raise LOG_MAX_TOTAL_SIZE_MB"))
	}

	return deleted, nil
}

// enforceActiveFileCeiling checks the file the logger holds open against
// LOG_MAX_ACTIVE_FILE_SIZE_MB and engages the bounded emergency policy when it
// is over the ceiling and nothing rotated it away.
//
// In the normal configuration the rotation writer enforces this at write time
// and this check never fires. It fires when rotation cannot act: ONLY_ONE_LOG_FILE
// writes to a single file with no rotation sink at all, and the guard must not
// pretend a bound is being enforced when it is not. Capping the bytes admitted
// is the honest response -- the file still cannot shrink, but it stops growing
// at the rate that got it here.
//
// Parameters:
//   - lg: the worker's logger.
//   - logDir: the directory holding the active file.
//   - maxActiveBytes: the ceiling; values <= 0 disable the check.
//
// Return values:
//   - error: wrapped failure when the active file cannot be inspected.
func enforceActiveFileCeiling(lg glog.Logger, logDir string, maxActiveBytes int64) error {
	if maxActiveBytes <= 0 {
		return nil
	}

	active := activeLogFile(logDir)
	if active == "" {
		releaseActiveFileCap(lg)
		return nil
	}

	info, err := os.Stat(active)
	if err != nil {
		if os.IsNotExist(err) {
			releaseActiveFileCap(lg)
			return nil
		}
		return errors.Wrapf(err, "stat active log file %s", active)
	}

	if info.Size() <= maxActiveBytes {
		releaseActiveFileCap(lg)
		return nil
	}

	engageActiveFileCap(lg, active, info.Size(), maxActiveBytes)
	return nil
}

// engageActiveFileCap turns on the bounded policy because the active file is
// over its ceiling.
//
// Parameters:
//   - lg: the worker's logger.
//   - path: the active file.
//   - size: its current size in bytes.
//   - maxActiveBytes: the ceiling it exceeded.
//
// Return values: none.
func engageActiveFileCap(lg glog.Logger, path string, size, maxActiveBytes int64) {
	engaged := diskEmergency.engage(metrics.LogSuppressReasonActiveFileCap)
	if !engaged && !activeFileReport.allow(time.Now().UTC()) {
		return
	}

	// This is an operator misconfiguration, not a server fault: the process is
	// doing exactly what it was told to do and reporting that the instruction
	// cannot be satisfied.
	lg.Warn("active log file is over its ceiling and cannot be rotated; "+
		"bounding application log output instead",
		zap.String("log_path", path),
		zap.Int64("active_file_bytes", size),
		zap.Int64("max_active_file_bytes", maxActiveBytes),
		zap.Int("log_emergency_max_bytes_per_sec", config.LogEmergencyMaxBytesPerSec),
		zap.String("hint", "ONLY_ONE_LOG_FILE disables rotation entirely; unset it to let "+
			"LOG_MAX_ACTIVE_FILE_SIZE_MB rotate the file by size"))
}

// releaseActiveFileCap turns off the active-file reason and reports what it
// suppressed.
//
// Parameters:
//   - lg: the worker's logger.
//
// Return values: none.
func releaseActiveFileCap(lg glog.Logger) {
	activeFileReport.reset()
	reportEmergencyRecovery(lg, diskEmergency.release(metrics.LogSuppressReasonActiveFileCap),
		metrics.LogSuppressReasonActiveFileCap)
}

// reportEmergencyRecovery emits the rate-limited recovery summary.
//
// Parameters:
//   - lg: the worker's logger.
//   - summary: what the closing emergency discarded.
//   - reason: the reason that was cleared, for the report.
//
// Return values: none.
func reportEmergencyRecovery(lg glog.Logger, summary emergencySummary, reason string) {
	if !summary.Report {
		return
	}

	lg.Warn("application log output left its bounded emergency policy",
		zap.String("reason", reason),
		zap.Int64("suppressed_lines", summary.Lines),
		zap.Int64("suppressed_bytes", summary.Bytes),
		zap.String("note", "the suppressed lines are lost; this is the gap they left in the log"))
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

	recoveryBytes := diskRecoveryFloor(minFreeBytes)

	free, err := freeDiskProbe(logDir)
	if err != nil {
		return 0, errors.Wrapf(err, "read free disk space for %s", logDir)
	}
	if int64(free) >= recoveryBytes {
		leaveDiskPressure(lg)
		return 0, nil
	}
	if int64(free) >= minFreeBytes && !diskEmergency.engaged.Load() && !levelEscalated.Load() {
		// Inside the hysteresis band and not currently degraded. Entering here
		// would mean entering and leaving on the same threshold, which is what
		// made the pre-W0.4 guard flap: one deletion or one temp file either
		// side of the floor toggled the whole log level.
		return 0, nil
	}

	if belowFloorReport.allow(time.Now().UTC()) {
		lg.Warn("free disk space below the configured floor, purging rotated logs",
			zap.String("log_dir", logDir),
			zap.Uint64("free_bytes", free),
			zap.Int64("min_free_bytes", minFreeBytes),
			zap.Int64("recovery_free_bytes", recoveryBytes))
	}

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
		if free, err = freeDiskProbe(logDir); err != nil {
			return deleted, errors.Wrapf(err, "read free disk space for %s", logDir)
		}
		if int64(free) >= recoveryBytes {
			leaveDiskPressure(lg)
			return deleted, nil
		}
	}

	enterDiskPressure(lg, logDir, free, minFreeBytes)
	return deleted, nil
}

// enterDiskPressure engages the bounded emergency policy and raises the log
// level, in that order.
//
// The order matters: the byte budget must already be in force before the
// escalation report is written, so the report itself is charged against the
// budget it announces.
//
// Parameters:
//   - lg: the worker's logger.
//   - logDir: the directory being guarded, for the report.
//   - free: currently free bytes.
//   - minFreeBytes: the floor that was not met.
//
// Return values: none.
func enterDiskPressure(lg glog.Logger, logDir string, free uint64, minFreeBytes int64) {
	diskEmergency.engage(metrics.LogSuppressReasonDiskPressure)
	escalateLogLevel(lg, logDir, free, minFreeBytes)
}

// leaveDiskPressure releases the bounded emergency policy, restores the log
// level, and reports what was discarded.
//
// Parameters:
//   - lg: the worker's logger.
//
// Return values: none.
func leaveDiskPressure(lg glog.Logger) {
	summary := diskEmergency.release(metrics.LogSuppressReasonDiskPressure)
	belowFloorReport.reset()
	restoreLogLevel(lg)
	reportEmergencyRecovery(lg, summary, metrics.LogSuppressReasonDiskPressure)
}

// activeLogFile returns the exact pathname the local writer currently holds
// open. It never guesses from directory timestamps: that guess can select a
// size-rotated sibling and let the retention guard unlink the live file.
//
// Parameters:
//   - logDir: the directory being swept.
//
// Return values:
//   - string: the active file path, or "" when the directory holds none.
func activeLogFile(logDir string) string {
	activeLogFiles.RLock()
	path := activeLogFiles.paths[filepath.Clean(logDir)]
	activeLogFiles.RUnlock()
	return path
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
// On its own this is NOT a bound -- warn and error remain fully enabled, and an
// error storm still fills the volume -- which is why the caller engages the
// emergency byte budget first (W0.4).
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
