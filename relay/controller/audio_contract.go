package controller

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// normalizeAudioWire rewrites only the authoritative model and documented
// provider aliases. Files, extension fields, explicit false and large integers
// remain unchanged. The normalized cache is shared by metering and dispatch.
func normalizeAudioWire(c *gin.Context, mode, channel int, actualModel string, tts *openai.TextToSpeechRequest) error {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "read audio request")
	}
	var wire []byte
	contentType := c.GetHeader("Content-Type")
	if mode == relaymode.AudioSpeech {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return errors.Wrap(err, "decode speech request")
		}
		if fields == nil {
			return errors.New("speech body must be a JSON object")
		}
		if raw, ok := fields["extra_body"]; ok {
			var extra map[string]json.RawMessage
			if err := json.Unmarshal(raw, &extra); err != nil {
				return errors.Wrap(err, "decode audio extra_body")
			}
			for key, value := range extra {
				if key == "model" || key == "input" {
					return errors.New("extra_body cannot override audio model or metered input")
				}
				if _, present := fields[key]; !present {
					fields[key] = value
				}
			}
			delete(fields, "extra_body")
		}
		modelJSON, err := json.Marshal(actualModel)
		if err != nil {
			return errors.Wrap(err, "encode audio model")
		}
		fields["model"] = modelJSON
		if channel == channeltype.Mistral {
			if raw, ok := fields["stream"]; ok {
				var stream bool
				if err := json.Unmarshal(raw, &stream); err != nil {
					return errors.Wrap(err, "decode speech stream")
				}
				if stream {
					return errors.New("Mistral standard audio/speech currently requires stream=false")
				}
			}
			var voice, voiceID, reference string
			for key, target := range map[string]*string{"voice": &voice, "voice_id": &voiceID, "ref_audio": &reference} {
				if raw, ok := fields[key]; ok {
					if err := json.Unmarshal(raw, target); err != nil {
						return errors.Wrapf(err, "decode %s", key)
					}
				}
			}
			if voice != "" && voiceID != "" && voice != voiceID {
				return errors.New("voice and voice_id conflict")
			}
			if voiceID == "" {
				voiceID = voice
			}
			if (voiceID == "") == (reference == "") {
				return errors.New("Mistral speech requires exactly one voice (voice_id) or ref_audio")
			}
			if voiceID != "" {
				encoded, err := json.Marshal(voiceID)
				if err != nil {
					return errors.Wrap(err, "encode voice_id")
				}
				fields["voice_id"] = encoded
			}
			delete(fields, "voice")
			if raw, ok := fields["speed"]; ok {
				var speed float64
				if err := json.Unmarshal(raw, &speed); err != nil {
					return errors.Wrap(err, "decode speech speed")
				}
				if speed != 1 {
					return errors.New("Mistral speech does not support the standard speed parameter")
				}
				delete(fields, "speed")
			}
			format := tts.ResponseFormat
			if raw, ok := fields["response_format"]; ok {
				if err := json.Unmarshal(raw, &format); err != nil {
					return errors.Wrap(err, "decode response_format")
				}
			}
			tts.ResponseFormat = format
			if format == "" {
				format = "mp3"
				tts.ResponseFormat = format
			}
			switch format {
			case "mp3", "wav", "flac", "opus":
			default:
				return errors.New("Mistral standard speech supports mp3, wav, flac, or opus; native float32 PCM is not standard PCM16")
			}
			encoded, err := json.Marshal(format)
			if err != nil {
				return errors.Wrap(err, "encode response_format")
			}
			fields["response_format"] = encoded
		}
		if channel == channeltype.Groq && strings.HasPrefix(actualModel, "canopylabs/orpheus-") {
			if utf8.RuneCountInString(tts.Input) > 200 {
				return errors.New("Orpheus speech input exceeds 200 characters")
			}
			var format string
			if raw, ok := fields["response_format"]; ok {
				if err := json.Unmarshal(raw, &format); err != nil {
					return errors.Wrap(err, "decode Orpheus response_format")
				}
			}
			if format != "" && format != "wav" {
				return errors.New("Orpheus supports WAV output only")
			}
			fields["response_format"] = json.RawMessage(`"wav"`)
			tts.ResponseFormat = "wav"
		}
		wire, err = json.Marshal(fields)
		if err != nil {
			return errors.Wrap(err, "encode normalized speech request")
		}
	} else {
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return errors.Wrap(err, "decode audio Content-Type")
		}
		if mediaType != "multipart/form-data" || params["boundary"] == "" {
			return errors.New("standard transcription requires a multipart file upload")
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		var out bytes.Buffer
		writer := multipart.NewWriter(&out)
		models, files := 0, 0
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return errors.Wrap(err, "read audio form part")
			}
			if part.FormName() == "url" || part.FormName() == "file_url" || part.FormName() == "audio_url" {
				return errors.New("a metered file upload cannot also select a remote audio URL")
			}
			if part.FormName() == "model" {
				models++
				if models > 1 {
					return errors.New("audio request contains duplicate model fields")
				}
				if err := part.Close(); err != nil {
					return errors.Wrap(err, "close model form part")
				}
				continue
			}
			if part.FormName() == "file" {
				files++
			}
			target, err := writer.CreatePart(part.Header)
			if err != nil {
				return errors.Wrap(err, "write audio form part")
			}
			if _, err := io.Copy(target, part); err != nil {
				return errors.Wrap(err, "copy audio form part")
			}
			if err := part.Close(); err != nil {
				return errors.Wrap(err, "close audio form part")
			}
		}
		if files != 1 {
			return errors.New("audio request requires exactly one file")
		}
		if err := writer.WriteField("model", actualModel); err != nil {
			return errors.Wrap(err, "write mapped audio model")
		}
		if err := writer.Close(); err != nil {
			return errors.Wrap(err, "close audio multipart body")
		}
		wire = out.Bytes()
		contentType = writer.FormDataContentType()
		if c.Request.MultipartForm != nil {
			if err := c.Request.MultipartForm.RemoveAll(); err != nil {
				return errors.Wrap(err, "remove parsed audio form temporary files")
			}
		}
		c.Request.MultipartForm = nil
		c.Request.Form = nil
		c.Request.PostForm = nil
	}
	c.Set(ctxkey.KeyRequestBody, wire)
	c.Request.Body = io.NopCloser(bytes.NewReader(wire))
	c.Request.ContentLength = int64(len(wire))
	c.Request.Header.Set("Content-Type", contentType)
	c.Request.Header.Set("Content-Length", strconv.Itoa(len(wire)))
	c.Set(ctxkey.ContentType, contentType)
	return nil
}

// decimalQuotaProduct multiplies nonnegative finite quantities as decimal
// rationals and rounds once, preventing floating-point noise and int64 overflow.
func decimalQuotaProduct(values ...float64) (int64, error) {
	return decimalQuotaRate(1, values...)
}

// decimalQuotaRate divides the decimal product by a positive unit denomination
// before rounding once. Hour/minute tariffs therefore never acquire a repeating
// per-second float and cannot spuriously charge an extra quota unit.
func decimalQuotaRate(denominator float64, values ...float64) (int64, error) {
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0, errors.New("billing rate denominator must be finite and positive")
	}
	scale, ok := new(big.Rat).SetString(strconv.FormatFloat(denominator, 'f', -1, 64))
	if !ok {
		return 0, errors.New("invalid billing denominator")
	}
	cost := new(big.Rat).Inv(scale)
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return 0, errors.New("billing quantities must be finite and nonnegative")
		}
		rational, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
		if !ok {
			return 0, errors.New("invalid decimal billing quantity")
		}
		cost.Mul(cost, rational)
	}
	result, rem := new(big.Int), new(big.Int)
	result.QuoRem(cost.Num(), cost.Denom(), rem)
	if rem.Sign() != 0 {
		result.Add(result, big.NewInt(1))
	}
	if !result.IsInt64() {
		return 0, errors.New("audio charge exceeds supported quota range")
	}
	return result.Int64(), nil
}

// quoteAudioInput returns the final input-based charge and diagnostic unit label.
// Direct tariffs override tokenizer approximations. A ratio-only administrator
// override retains its historic unit rate instead of silently using provider USD.
func quoteAudioInput(mode int, text string, seconds, tokensPerSecond, modelRatio, group float64, cfg *adaptor.AudioPricingConfig, ratioOverride bool) (int64, string, error) {
	if mode == relaymode.AudioSpeech {
		quantity := utf8.RuneCountInString(text)
		unit := "characters"
		if cfg != nil && cfg.InputUnit == "utf8_bytes" {
			quantity = len(text)
			unit = "utf8_bytes"
		}
		if quantity == 0 {
			return 0, unit, errors.New("speech input is empty")
		}
		if cfg != nil && (cfg.InputUnit == "characters" || cfg.InputUnit == "utf8_bytes") && !ratioOverride {
			quota, err := decimalQuotaRate(cfg.InputPriceQuantity, float64(quantity), cfg.InputPriceUsd, float64(billingratio.QuotaPerUsd), group)
			return quota, unit, err
		}
		quota, err := decimalQuotaProduct(float64(quantity), modelRatio, group)
		return quota, unit + " (configured legacy unit ratio)", err
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, "seconds", errors.New("audio duration must be finite and positive")
	}
	if cfg != nil {
		if math.IsNaN(cfg.MinimumBillableSeconds) || math.IsInf(cfg.MinimumBillableSeconds, 0) || cfg.MinimumBillableSeconds < 0 || math.IsNaN(cfg.BillingIncrementSeconds) || math.IsInf(cfg.BillingIncrementSeconds, 0) || cfg.BillingIncrementSeconds < 0 {
			return 0, "seconds", errors.New("invalid audio billing minimum or increment")
		}
		seconds = max(seconds, cfg.MinimumBillableSeconds)
		if cfg.BillingIncrementSeconds > 0 {
			count, err := decimalQuotaRate(cfg.BillingIncrementSeconds, seconds)
			if err != nil {
				return 0, "seconds", err
			}
			// Reconstruct the rounded duration as a decimal, not count*increment.
			step, _ := new(big.Rat).SetString(strconv.FormatFloat(cfg.BillingIncrementSeconds, 'f', -1, 64))
			step.Mul(step, new(big.Rat).SetInt64(count))
			seconds, _ = step.Float64()
		}
		if cfg.InputUnit == "seconds" && !ratioOverride {
			quota, err := decimalQuotaRate(cfg.InputPriceQuantity, seconds, cfg.InputPriceUsd, float64(billingratio.QuotaPerUsd), group)
			return quota, "seconds", err
		}
		if cfg.UsdPerSecond > 0 && !ratioOverride {
			quota, err := decimalQuotaProduct(seconds, cfg.UsdPerSecond, float64(billingratio.QuotaPerUsd), group)
			return quota, "seconds", err
		}
	}
	quota, err := decimalQuotaProduct(seconds, tokensPerSecond, modelRatio, group)
	return quota, "seconds (configured token conversion)", err
}

// normalizeAudioResponse detects JSON error envelopes and converts Mistral's
// base64 receipt into the standard binary speech response. It never changes
// arbitrary provider binary payloads or attaches credentials to generated media.
func normalizeAudioResponse(resp *http.Response, mode, channel int, format string) error {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	mistralSpeech := channel == channeltype.Mistral && mode == relaymode.AudioSpeech
	if !mistralSpeech && !strings.Contains(contentType, "json") {
		if strings.Contains(contentType, "text/html") {
			return errors.New("unexpected HTML audio response")
		}
		if mode == relaymode.AudioSpeech {
			reader := bufio.NewReader(resp.Body)
			if _, err := reader.Peek(1); err != nil {
				return errors.Wrap(err, "empty or unreadable speech audio")
			}
			resp.Body = &audioReadCloser{Reader: reader, Closer: resp.Body}
		}
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<20)+1))
	if err != nil {
		return errors.Wrap(err, "read audio response")
	}
	if len(body) > 64<<20 {
		return errors.New("audio response exceeds 64 MiB")
	}
	if err := resp.Body.Close(); err != nil {
		return errors.Wrap(err, "close audio response")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return errors.Wrap(err, "decode audio JSON response")
	}
	if raw, ok := payload["error"]; ok && string(raw) != "null" {
		return errors.New("upstream returned an audio error envelope")
	}
	if !mistralSpeech {
		if mode == relaymode.AudioSpeech {
			return errors.New("binary speech endpoint returned JSON instead of audio")
		}
		var text string
		raw, exists := payload["text"]
		if !exists || string(raw) == "null" {
			return errors.New("transcription response is missing text")
		}
		if err := json.Unmarshal(raw, &text); err != nil {
			return errors.Wrap(err, "decode transcription text")
		}
		return nil
	}
	var audio string
	if err := json.Unmarshal(payload["audio_data"], &audio); err != nil {
		return errors.Wrap(err, "decode Mistral audio_data")
	}
	decoded, err := base64.StdEncoding.DecodeString(audio)
	if err != nil {
		return errors.Wrap(err, "decode Mistral speech audio")
	}
	if len(decoded) == 0 {
		return errors.New("Mistral returned empty speech audio")
	}
	mimeType := map[string]string{"mp3": "audio/mpeg", "wav": "audio/wav", "flac": "audio/flac", "opus": "audio/ogg"}[format]
	if mimeType == "" {
		return errors.New("unsupported Mistral speech format")
	}
	resp.Body = io.NopCloser(bytes.NewReader(decoded))
	resp.ContentLength = int64(len(decoded))
	resp.Header.Set("Content-Type", mimeType)
	resp.Header.Set("Content-Length", strconv.Itoa(len(decoded)))
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("ETag")
	return nil
}

// audioReadCloser retains the upstream closer when peeking into binary speech.
// Its Reader preserves every byte, while Closer releases the actual connection.
type audioReadCloser struct {
	io.Reader
	io.Closer
}
