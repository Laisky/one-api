package gemini

import (
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/gif"  // Registers GIF dimensions for visual input bounds.
	_ "image/jpeg" // Registers JPEG dimensions for visual input bounds.
	_ "image/png"  // Registers PNG dimensions for visual input bounds.
	"math"
	"mime"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	_ "golang.org/x/image/webp" // Registers WebP dimensions for visual input bounds.

	"github.com/Laisky/one-api/relay/realtime"
)

// Conservative tokenization bounds used to fund Live work BEFORE forwarding it.
// Authoritative receipts replace these estimates; the bounds only have to be at
// least the provider's count. Sources:
//   - https://ai.google.dev/gemini-api/docs/tokens (audio 32 tokens/second;
//     images 258 tokens per 768x768 tile)
//   - https://ai.google.dev/gemini-api/docs/media-resolution (largest Gemini 3
//     per-image allowance: 2,240 tokens at ultra-high resolution)
//   - https://ai.google.dev/gemini-api/docs/live-guide (raw little-endian
//     16-bit PCM; input natively 16 kHz with audio/pcm;rate=N, output 24 kHz)
//
// Text never exceeds one token per UTF-8 byte, because byte-fallback tokenizers
// emit at most one token per byte.
const (
	liveAudioTokensPerSecond    = 32
	liveDefaultInputSampleRate  = 16000
	liveOutputSampleRate        = 24000
	liveImageTilePixels         = 768
	liveImageTileTokens         = 258
	liveImageTokenFloor         = 2240
	liveDefaultTurnOutputTokens = 3000
)

// estimateLiveClientFrame bounds the input tokens of one validated native client
// operation. Parameters: data is a frame accepted by validateLiveClientFrame.
// Returns: modality-specific input tokens, or an error wrapping
// realtime.ErrUnpriceableInput when the work cannot be bounded locally.
func estimateLiveClientFrame(data []byte) (realtime.Estimate, error) {
	root, err := liveObject(data)
	if err != nil {
		return realtime.Estimate{}, err
	}
	if raw := root["realtimeInput"]; raw != nil {
		return estimateLiveRealtimeInput(raw)
	}
	// clientContent turns, toolResponse results and setup carry Content parts.
	return estimateLiveContent(data)
}

// estimateLiveRealtimeInput bounds one realtimeInput object. Parameters: raw is
// its JSON value. Returns: input tokens; activity controls carry no input tokens.
func estimateLiveRealtimeInput(raw json.RawMessage) (realtime.Estimate, error) {
	var input struct {
		Text   *string          `json:"text"`
		Audio  *json.RawMessage `json:"audio"`
		Video  *json.RawMessage `json:"video"`
		Chunks []liveBlob       `json:"mediaChunks"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return realtime.Estimate{}, errors.Wrap(ErrLiveProtocol, "invalid realtime input")
	}
	var total realtime.Estimate
	if input.Text != nil {
		total.Text = int64(len(*input.Text))
	}
	blobs := append([]liveBlob(nil), input.Chunks...)
	for _, value := range []*json.RawMessage{input.Audio, input.Video} {
		if value == nil {
			continue
		}
		var blob liveBlob
		if err := json.Unmarshal(*value, &blob); err != nil {
			return realtime.Estimate{}, errors.Wrap(ErrLiveProtocol, "invalid realtime media")
		}
		blobs = append(blobs, blob)
	}
	for _, blob := range blobs {
		cost, err := estimateLiveBlob(blob, liveDefaultInputSampleRate)
		if err != nil {
			return realtime.Estimate{}, err
		}
		total = total.Add(cost)
	}
	return total, nil
}

// liveBlob is a native inline media object. Protobuf JSON accepts both the
// lowerCamelCase and original snake_case field names, so both are read.
type liveBlob struct {
	MimeType  string `json:"mimeType"`
	MimeSnake string `json:"mime_type"`
	Data      string `json:"data"`
}

// mime returns the declared media type. Parameters: none. Returns: either spelling.
func (b liveBlob) mime() string {
	if b.MimeType != "" {
		return b.MimeType
	}
	return b.MimeSnake
}

// estimateLiveBlob bounds one inline media object. Parameters: blob is the
// media and defaultRate is the PCM sample rate assumed without a rate
// parameter. Returns: input tokens or an unpriceable-input error. Unknown media
// types fail closed instead of being forwarded at an unbounded price.
func estimateLiveBlob(blob liveBlob, defaultRate int64) (realtime.Estimate, error) {
	mediaType, params, err := mime.ParseMediaType(blob.mime())
	if err != nil {
		return realtime.Estimate{}, errors.Wrap(realtime.ErrUnpriceableInput, "invalid inline media type")
	}
	decoded := base64DecodedBound(blob.Data)
	switch {
	case mediaType == "audio/pcm":
		rate := defaultRate
		if value, ok := params["rate"]; ok {
			rate, err = strconv.ParseInt(value, 10, 64)
			if err != nil || rate <= 0 {
				return realtime.Estimate{}, errors.Wrap(realtime.ErrUnpriceableInput, "invalid PCM sample rate")
			}
		}
		return realtime.Estimate{Audio: pcmTokens(decoded, rate)}, nil
	case strings.HasPrefix(mediaType, "image/"):
		tokens, err := imageTokens(blob.Data)
		if err != nil {
			return realtime.Estimate{}, err
		}
		return realtime.Estimate{Image: tokens}, nil
	case strings.HasPrefix(mediaType, "text/"):
		return realtime.Estimate{Text: decoded}, nil
	default:
		return realtime.Estimate{}, errors.Wrap(realtime.ErrUnpriceableInput, "unsupported inline media type")
	}
}

// base64DecodedBound returns an upper bound on the decoded size of data.
// Parameters: data is standard or URL-safe base64. Returns: a byte count.
func base64DecodedBound(data string) int64 {
	return (int64(len(data))/4 + 1) * 3
}

// pcmTokens bounds 16-bit mono PCM tokens. Parameters: size is the byte count
// and rate the declared sample rate. Returns: ceil(seconds * tokens/second).
// Multi-channel audio is shorter than assumed, so the bound stays conservative.
func pcmTokens(size, rate int64) int64 {
	tokens := math.Ceil(float64(size) * liveAudioTokensPerSecond / (2 * float64(rate)))
	if tokens >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(tokens)
}

// imageTokens bounds one image from its encoded dimensions. Parameters: data is
// base64 image bytes. Returns: tokens for every 768-pixel tile, never fewer than
// the largest per-image media-resolution allowance, or an unpriceable error.
func imageTokens(data string) (int64, error) {
	var config image.Config
	var err error
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		config, _, err = image.DecodeConfig(base64.NewDecoder(encoding, strings.NewReader(data)))
		if err == nil {
			break
		}
	}
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, errors.Wrap(realtime.ErrUnpriceableInput, "image dimensions cannot be determined")
	}
	tiles := int64((config.Width+liveImageTilePixels-1)/liveImageTilePixels) *
		int64((config.Height+liveImageTilePixels-1)/liveImageTilePixels)
	return max(tiles*liveImageTileTokens, liveImageTokenFloor), nil
}

// estimateLiveContent bounds frames whose payload is native Content: setup,
// clientContent and toolResponse. Every byte except inline media data is priced
// as text; inline media is priced by modality and provider-fetched file
// references fail closed. Parameters: data is validated JSON. Returns: tokens.
func estimateLiveContent(data []byte) (realtime.Estimate, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return realtime.Estimate{}, errors.Wrap(ErrLiveProtocol, "invalid Live content")
	}
	var media realtime.Estimate
	var mediaBytes int64
	if err := walkLiveContent(value, &media, &mediaBytes); err != nil {
		return realtime.Estimate{}, err
	}
	media.Text += max(int64(len(data))-mediaBytes, 0)
	return media, nil
}

// walkLiveContent visits every nested object. Parameters: value is decoded JSON,
// media accumulates inline media tokens and mediaBytes the raw data excluded
// from text pricing. Returns: an error for unpriceable or malformed media.
func walkLiveContent(value any, media *realtime.Estimate, mediaBytes *int64) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "fileData", "file_data":
				return errors.Wrap(realtime.ErrUnpriceableInput, "provider-fetched file references cannot be priced before forwarding")
			case "inlineData", "inline_data":
				object, ok := child.(map[string]any)
				if !ok {
					return errors.Wrap(ErrLiveProtocol, "invalid inline media")
				}
				blob := liveBlob{}
				blob.MimeType, _ = object["mimeType"].(string)
				blob.MimeSnake, _ = object["mime_type"].(string)
				blob.Data, _ = object["data"].(string)
				cost, err := estimateLiveBlob(blob, liveDefaultInputSampleRate)
				if err != nil {
					return err
				}
				*media = media.Add(cost)
				*mediaBytes += int64(len(blob.Data))
			default:
				if err := walkLiveContent(child, media, mediaBytes); err != nil {
					return err
				}
			}
		}
	case []any:
		for _, child := range node {
			if err := walkLiveContent(child, media, mediaBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

// estimateLiveServerOutput bounds the output streamed by one trusted server
// frame. Parameters: data is validated provider JSON. Returns: output tokens of
// model text, thoughts, audio, transcription and function calls in the frame.
// Hidden thinking is not on the wire; per-turn allowances and receipts cover it.
func estimateLiveServerOutput(data []byte) realtime.Estimate {
	var event struct {
		Content *struct {
			Model *struct {
				Parts []struct {
					Text   string    `json:"text"`
					Inline *liveBlob `json:"inlineData"`
				} `json:"parts"`
			} `json:"modelTurn"`
			Output *struct {
				Text string `json:"text"`
			} `json:"outputTranscription"`
		} `json:"serverContent"`
		Tool json.RawMessage `json:"toolCall"`
	}
	if json.Unmarshal(data, &event) != nil {
		return realtime.Estimate{}
	}
	var total realtime.Estimate
	total.OutputText = int64(len(event.Tool))
	if event.Content == nil {
		return total
	}
	if event.Content.Output != nil {
		total.OutputText += int64(len(event.Content.Output.Text))
	}
	if event.Content.Model != nil {
		for _, part := range event.Content.Model.Parts {
			total.OutputText += int64(len(part.Text))
			if part.Inline != nil {
				rate := int64(liveOutputSampleRate)
				if _, params, err := mime.ParseMediaType(part.Inline.mime()); err == nil {
					if value, err := strconv.ParseInt(params["rate"], 10, 64); err == nil && value > 0 {
						rate = value
					}
				}
				// Provider output media is always billed; price unknown kinds as audio.
				total.OutputAudio += pcmTokens(base64DecodedBound(part.Inline.Data), rate)
			}
		}
	}
	return total
}

// liveTurnOutputAllowance returns the per-turn output reserve. Parameters: setup
// is the validated setup frame. Returns: generationConfig.maxOutputTokens when
// the client set a positive provider-enforced cap, otherwise the default reserve.
func liveTurnOutputAllowance(setup []byte) int64 {
	var frame struct {
		Setup struct {
			Generation struct {
				MaxOutputTokens json.Number `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		} `json:"setup"`
	}
	if json.Unmarshal(setup, &frame) != nil {
		return liveDefaultTurnOutputTokens
	}
	if value, err := frame.Setup.Generation.MaxOutputTokens.Int64(); err == nil && value > 0 {
		return value
	}
	return liveDefaultTurnOutputTokens
}
