package logger

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

const alertMessageByteLimit = 16 * 1024

var errAlertEncoderNoBuffer = errors.New("alert encoder returned no buffer")

// Redact takes a caller-selected private value without inspecting it and returns a visible replacement for every logging sink.
// Callers choose their application privacy policy; the generic alert transport never guesses which fields are sensitive.
func Redact(_ any) string { return "[REDACTED]" }

// NewAlertCore takes ctx, endpoint, alertType, token and limiter and returns the owned transport, its transparent ERROR-level Zap core, or a construction error.
// Callers must Close the alert and wait for Done before discarding its lifecycle context. Close cancels delivery rather than flushing.
func NewAlertCore(ctx context.Context, endpoint, alertType, token string, limiter glog.RateLimiter) (*glog.Alert, zapcore.Core, error) {
	alert, err := glog.NewAlert(ctx, endpoint, glog.WithAlertType(alertType), glog.WithAlertToken(token), glog.WithRateLimiter(limiter))
	if err != nil {
		return nil, nil, errors.Wrap(err, "create alert core transport")
	}
	cfg := zap.NewProductionEncoderConfig()
	cfg.MessageKey = "message"
	cfg.TimeKey = "time"
	cfg.EncodeTime = alertUTCEncodeTime
	return alert, &alertCore{alert: alert, encoder: zapcore.NewJSONEncoder(cfg)}, nil
}

// alertUTCEncodeTime takes a timestamp and primitive encoder, appends its UTC RFC3339Nano representation, and returns no value.
func alertUTCEncodeTime(value time.Time, encoder zapcore.PrimitiveArrayEncoder) {
	encoder.AppendString(value.UTC().Format(time.RFC3339Nano))
}

// alertCore preserves standard Zap encoder context and sends completed JSON entries through the existing alert queue.
type alertCore struct {
	alert   *glog.Alert
	encoder zapcore.Encoder
	fields  []zapcore.Field
}

// Enabled takes level and returns whether it meets the alert's existing ERROR floor.
func (*alertCore) Enabled(level zapcore.Level) bool { return level >= zapcore.ErrorLevel }

// With takes inherited fields and returns an independent encoder clone with ordinary Zap namespace and serialization semantics.
func (c *alertCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &alertCore{alert: c.alert, encoder: c.encoder.Clone(), fields: append(c.fields[:len(c.fields):len(c.fields)], fields...)}
	for _, field := range fields {
		field.AddTo(clone.encoder)
	}
	return clone
}

// Check takes an entry and checked entry, and returns the checked entry with this core added when its level is enabled.
func (c *alertCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

// Write takes an enabled entry and call-site fields and returns any encoding or queue-admission error.
func (c *alertCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	buffer, err := c.encoder.EncodeEntry(entry, fields)
	if buffer != nil {
		defer buffer.Free()
	}
	if err != nil {
		return errors.Wrap(err, "encode alert entry")
	}
	if buffer == nil {
		return errors.WithStack(errAlertEncoderNoBuffer)
	}
	message, err := boundedAlertMessage(buffer.Bytes())
	if err != nil {
		return err
	}
	return errors.Wrap(c.alert.Send(message), "enqueue alert entry")
}

// Sync takes no arguments and returns no error because the existing transport is asynchronous and has no flush API.
func (*alertCore) Sync() error { return nil }

// Fields takes no arguments and returns this core's inherited context as required by Zap's core interface.
func (c *alertCore) Fields() []zapcore.Field { return c.fields }

// boundedAlertMessage takes encoded JSON and returns it unchanged within the cap, or a valid JSON envelope with an explicit UTF-8-safe truncation preview.
func boundedAlertMessage(encoded []byte) (string, error) {
	if len(encoded) <= alertMessageByteLimit {
		return string(encoded), nil
	}
	envelope := struct {
		Truncated     bool   `json:"alert_truncated"`
		OriginalBytes int    `json:"original_bytes"`
		Preview       string `json:"preview"`
	}{Truncated: true, OriginalBytes: len(encoded)}
	low, high := 0, min(len(encoded), alertMessageByteLimit)
	var selected []byte
	for low <= high {
		middle := (low + high) / 2
		end := middle
		for end > 0 && !utf8.RuneStart(encoded[end]) {
			end--
		}
		envelope.Preview = string(encoded[:end])
		candidate, err := json.Marshal(envelope)
		if err != nil {
			return "", errors.Wrap(err, "encode alert truncation marker")
		}
		if len(candidate) <= alertMessageByteLimit {
			selected = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return string(selected), nil
}
