package logger

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	errors "github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
)

const (
	rotationScheme             = "oneapi-rotate"
	rotationFilenameDateLayout = "20060102"
	defaultLoggerName          = "one-api"
)

type rotationInterval string

const (
	rotationIntervalHourly rotationInterval = "hourly"
	rotationIntervalDaily  rotationInterval = "daily"
	rotationIntervalWeekly rotationInterval = "weekly"
)

func (ri rotationInterval) String() string {
	return string(ri)
}

func (ri rotationInterval) valid() bool {
	switch ri {
	case rotationIntervalHourly, rotationIntervalDaily, rotationIntervalWeekly:
		return true
	default:
		return false
	}
}

func (ri rotationInterval) windowBounds(ts time.Time) (time.Time, time.Time) {
	now := ts.UTC()
	switch ri {
	case rotationIntervalHourly:
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
		return start, start.Add(time.Hour)
	case rotationIntervalDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.Add(24 * time.Hour)
	case rotationIntervalWeekly:
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		daysSinceMonday := (int(midnight.Weekday()) - int(time.Monday) + 7) % 7
		start := midnight.AddDate(0, 0, -daysSinceMonday)
		return start, start.Add(7 * 24 * time.Hour)
	default:
		panic(fmt.Sprintf("unsupported rotation interval: %q", ri))
	}
}

var (
	registerRotationSinkOnce sync.Once
	rotationSinkErr          error
	rotationNow              = defaultRotationNow
)

func defaultRotationNow() time.Time {
	return time.Now().UTC()
}

func ensureRotationSinkRegistered() error {
	registerRotationSinkOnce.Do(func() {
		rotationSinkErr = zap.RegisterSink(rotationScheme, createRotationSink)
	})

	if rotationSinkErr != nil {
		return errors.Wrap(rotationSinkErr, "register rotation sink")
	}

	return nil
}

func buildRotationSinkURL(path string, interval rotationInterval, retentionDays int) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.Errorf("rotation path must not be empty")
	}

	if !interval.valid() {
		return "", errors.Errorf("rotation interval %q is not supported", interval)
	}

	if retentionDays < 0 {
		return "", errors.Errorf("rotation retention days must be >= 0")
	}

	if err := ensureRotationSinkRegistered(); err != nil {
		return "", errors.Wrap(err, "ensure rotation sink registered")
	}

	values := url.Values{}
	values.Set("path", filepath.Clean(path))
	values.Set("interval", interval.String())
	if retentionDays > 0 {
		values.Set("retention_days", strconv.Itoa(retentionDays))
	}

	return fmt.Sprintf("%s://?%s", rotationScheme, values.Encode()), nil
}

func createRotationSink(u *url.URL) (zap.Sink, error) {
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.Wrap(err, "parse rotation sink query")
	}

	targetPath := query.Get("path")
	if targetPath == "" {
		if u.Path != "" {
			targetPath = u.Path
		} else {
			return nil, errors.Errorf("rotation sink missing path")
		}
	}

	intervalRaw := query.Get("interval")
	interval, err := parseRotationInterval(intervalRaw)
	if err != nil {
		return nil, errors.Wrap(err, "parse rotation interval")
	}

	retentionDays := 0
	if raw := query.Get("retention_days"); raw != "" {
		retentionDays, err = strconv.Atoi(raw)
		if err != nil {
			return nil, errors.Wrap(err, "parse rotation retention days")
		}
		if retentionDays < 0 {
			return nil, errors.Errorf("rotation retention days must be >= 0")
		}
	}

	writer, err := newRotationWriter(filepath.Clean(targetPath), interval, retentionDays)
	if err != nil {
		return nil, errors.Wrap(err, "create rotation writer")
	}

	return writer, nil
}

func parseRotationInterval(raw string) (rotationInterval, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "hourly":
		return rotationIntervalHourly, nil
	case "", "daily":
		return rotationIntervalDaily, nil
	case "weekly":
		return rotationIntervalWeekly, nil
	default:
		return "", errors.Errorf("unsupported rotation interval: %s", raw)
	}
}

type rotationWriter struct {
	mu            sync.Mutex
	file          *os.File
	baseDir       string
	loggerName    string
	extension     string
	activePath    string
	interval      rotationInterval
	retentionDays int
	windowStart   time.Time
	nextCutover   time.Time
	now           func() time.Time
}

func newRotationWriter(path string, interval rotationInterval, retentionDays int) (*rotationWriter, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.Errorf("rotation path must not be empty")
	}
	if !interval.valid() {
		return nil, errors.Errorf("rotation interval %q is not supported", interval)
	}
	if retentionDays < 0 {
		return nil, errors.Errorf("rotation retention days must be >= 0")
	}

	cleaned := filepath.Clean(path)
	baseDir := filepath.Dir(cleaned)
	base := filepath.Base(cleaned)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = defaultLoggerName
	}
	if ext == "" {
		ext = ".log"
	}

	sanitized := sanitizeLoggerComponent(name)

	return &rotationWriter{
		baseDir:       baseDir,
		loggerName:    sanitized,
		extension:     ext,
		interval:      interval,
		retentionDays: retentionDays,
		now:           rotationNow,
	}, nil
}

func (w *rotationWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	current := w.now()
	if err := w.ensureFile(current); err != nil {
		return 0, errors.Wrap(err, "prepare log file")
	}

	written, err := w.file.Write(p)
	if err != nil {
		return written, errors.Wrap(err, "write log file")
	}

	return written, nil
}

func (w *rotationWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if err := w.file.Sync(); err != nil {
		return errors.Wrap(err, "sync log file")
	}

	return nil
}

func (w *rotationWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if err := w.file.Close(); err != nil {
		return errors.Wrap(err, "close log file")
	}

	w.file = nil
	w.activePath = ""
	return nil
}

func (w *rotationWriter) ensureFile(ts time.Time) error {
	if w.file == nil {
		start, next := w.interval.windowBounds(ts)
		if err := w.openNewFile(start, next); err != nil {
			return errors.Wrap(err, "open new log file")
		}
		return errors.Wrap(w.purgeExpired(start), "purge expired log files")
	}

	if ts.Before(w.nextCutover) {
		return nil
	}

	start, next := w.interval.windowBounds(ts)
	return w.rotate(start, next)
}

func (w *rotationWriter) openNewFile(start, next time.Time) error {
	if err := ensureDir(w.baseDir); err != nil {
		return errors.Wrap(err, "ensure log directory")
	}

	path := w.activePathFor(start)
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	w.file = handle
	w.windowStart = start
	w.nextCutover = next
	w.activePath = path
	return nil
}

func (w *rotationWriter) activePathFor(start time.Time) string {
	dateStamp := start.Format(rotationFilenameDateLayout)
	filename := fmt.Sprintf("%s-%s%s", w.loggerName, dateStamp, w.extension)
	return filepath.Join(w.baseDir, filename)
}

func (w *rotationWriter) rotate(start, next time.Time) error {
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return errors.Wrap(err, "sync rotating log file")
		}
		if err := w.file.Close(); err != nil {
			return errors.Wrap(err, "close rotating log file")
		}

		w.file = nil
		w.activePath = ""
	}

	if err := w.openNewFile(start, next); err != nil {
		return errors.Wrap(err, "open new log file during rotation")
	}

	return errors.Wrap(w.purgeExpired(start), "purge expired log files during rotation")
}

func (w *rotationWriter) purgeExpired(currentStart time.Time) error {
	if w.retentionDays <= 0 {
		return nil
	}

	threshold := currentStart.AddDate(0, 0, -w.retentionDays)
	entries, err := os.ReadDir(w.baseDir)
	if err != nil {
		return errors.Wrap(err, "list log directory")
	}

	activeName := filepath.Base(w.activePath)
	prefix := w.loggerName + "-"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name == activeName {
			continue
		}

		if !strings.HasPrefix(name, prefix) || filepath.Ext(name) != w.extension {
			continue
		}

		dateComponent := strings.TrimSuffix(strings.TrimPrefix(name, prefix), w.extension)
		if len(dateComponent) != len(rotationFilenameDateLayout) {
			continue
		}

		ts, err := time.ParseInLocation(rotationFilenameDateLayout, dateComponent, time.UTC)
		if err != nil {
			continue
		}

		if ts.Before(threshold) {
			target := filepath.Join(w.baseDir, name)
			if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
				return errors.Wrapf(removeErr, "remove expired log %s", target)
			}
		}
	}

	return nil
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.Wrap(err, "create log directory")
	}

	return nil
}

func sanitizeLoggerComponent(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = defaultLoggerName
	}

	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}

	res := strings.Trim(b.String(), "-_.")
	if res == "" {
		return defaultLoggerName
	}

	return res
}
