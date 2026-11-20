package controller

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// videoDurationText is the accepted string form of a duration alias: unsigned
// decimal seconds such as "4" or "7.5". Signs, exponents, hex, whitespace and
// NaN/Infinity are refused so billing parses exactly the value a provider reads,
// and the digit bounds keep a hostile value from becoming an absurd quote.
var videoDurationText = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,9})?$`)

// videoDuration is one client duration alias. The OpenAI Videos contract types
// `seconds` as a string enum ("4", "8", "12") and the official SDKs send it as a
// multipart form field, while legacy gateway clients send a JSON number; both
// decode to the same value. The client's bytes are forwarded unchanged.
type videoDuration float64

// UnmarshalJSON decodes a JSON number or a decimal string into d.
//
// Parameters:
//   - data: the raw JSON value of the alias; null never reaches this method
//     because the aliases are pointer fields.
//
// Returns:
//   - error: a wrapped error when the value is neither a finite JSON number
//     nor a decimal string accepted by videoDurationText.
func (d *videoDuration) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return errors.Wrap(err, "decode video duration string")
		}
		return d.UnmarshalParam(text)
	}
	if len(trimmed) == 0 || (trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9')) {
		return errors.New("video duration must be a number or a decimal string such as \"4\"")
	}
	value, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil {
		return errors.Wrap(err, "parse video duration number")
	}
	*d = videoDuration(value)
	return nil
}

// UnmarshalParam decodes a form field (or the content of a JSON string) into
// d. gin's form binding calls it for multipart and urlencoded bodies.
//
// Parameters:
//   - param: the raw field text.
//
// Returns:
//   - error: a wrapped error when param is not unsigned decimal seconds.
func (d *videoDuration) UnmarshalParam(param string) error {
	if !videoDurationText.MatchString(param) {
		return errors.New("video duration string must be decimal seconds such as \"4\"")
	}
	value, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return errors.Wrap(err, "parse video duration string")
	}
	*d = videoDuration(value)
	return nil
}

// videoRequestWire decodes a video creation body. It embeds the shared DTO and
// shadows its three duration aliases (encoding/json prefers the shallower field
// of the same name) with videoDuration, so JSON strings and numbers and form
// fields all decode. Shadowed form tags are also bound into the embedded
// floats; decodeVideoRequest overwrites them from the strict values here.
type videoRequestWire struct {
	relaymodel.VideoRequest
	Seconds         *videoDuration `json:"seconds,omitempty" form:"seconds"`
	Duration        *videoDuration `json:"duration,omitempty" form:"duration"`
	DurationSeconds *videoDuration `json:"duration_seconds,omitempty" form:"duration_seconds"`
}

// decodeVideoRequest reads a video creation body (JSON, multipart or
// urlencoded) into the shared video DTO for both POST /v1/videos and the
// durable async controller. Every duration alias the client sends must be
// finite and positive, and all of them must agree: providers read one alias
// (OpenAI reads `seconds`) while billing resolves another by priority, so a
// disagreement could otherwise charge for less than the provider renders.
// An omitted duration stays unset and is never defaulted here: OpenAI's
// documented 4-second default described its own Videos API, retired on
// 2026-09-24, and the hosts still behind this route may render a different
// default length, so duration-billed models refuse the omission downstream.
// The request body itself is left untouched for forwarding.
//
// Parameters:
//   - c: the request context whose reusable body is decoded.
//
// Returns:
//   - *relaymodel.VideoRequest: the decoded request with normalized aliases.
//   - *relaymodel.ErrorWithStatusCode: a 400 invalid_video_request for a body or
//     alias that cannot be decoded, or invalid_video_duration for a
//     non-positive or conflicting duration; nil on success.
func decodeVideoRequest(c *gin.Context) (*relaymodel.VideoRequest, *relaymodel.ErrorWithStatusCode) {
	wire := &videoRequestWire{}
	if err := common.UnmarshalBodyReusable(c, wire); err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "parse video request"), "invalid_video_request", http.StatusBadRequest)
	}
	request := wire.VideoRequest
	aliases := []struct {
		name   string
		value  *videoDuration
		target **float64
	}{
		{"duration_seconds", wire.DurationSeconds, &request.DurationSeconds},
		{"seconds", wire.Seconds, &request.Seconds},
		{"duration", wire.Duration, &request.Duration},
	}
	agreed, present := 0.0, 0
	for _, alias := range aliases {
		*alias.target = nil
		if alias.value == nil {
			continue
		}
		value := float64(*alias.value)
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return nil, openai.ErrorWrapper(errors.Errorf("video %s must be a positive number of seconds", alias.name), "invalid_video_duration", http.StatusBadRequest)
		}
		if present > 0 && value != agreed {
			return nil, openai.ErrorWrapper(errors.New("conflicting video duration fields; send only seconds"), "invalid_video_duration", http.StatusBadRequest)
		}
		agreed, present = value, present+1
		*alias.target = &value
	}
	gmw.GetLogger(c).Debug("decoded video request duration",
		zap.Float64("duration_seconds", agreed),
		zap.Int("duration_aliases", present))
	return &request, nil
}
