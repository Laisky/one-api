package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/pkoukk/tiktoken-go"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	imgutil "github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// defaultEncodingName is the tiktoken encoding used as a fallback for any model
// whose encoding cannot be resolved. cl100k_base covers gpt-3.5/gpt-4 and is the
// most broadly compatible choice for the many non-OpenAI models routed through
// this token-counting path.
const defaultEncodingName = "cl100k_base"

// encoderByName caches one *tiktoken.Tiktoken per ENCODING name (not per model).
// Models that share an encoding (e.g. gpt-3.5-turbo and gpt-4 both use
// cl100k_base) therefore share a single CoreBPE instead of each building its own
// (~7 MiB of decoder + sortedTokenBytes tables wasted per duplicate). Encoders
// are loaded lazily on first use, so an encoding that is never requested (e.g.
// o200k_base on a deployment that never serves gpt-4o-class models) is never
// built and never occupies the heap.
var (
	encoderMu     sync.RWMutex
	encoderByName = make(map[string]*tiktoken.Tiktoken)
)

// InitTokenEncoders warms the default encoder so a missing/offline BPE dictionary
// fails fast at startup rather than on the first request. All other encodings are
// loaded on demand by getTokenEncoder.
func InitTokenEncoders() {
	if _, err := loadEncoder(defaultEncodingName); err != nil {
		panic(fmt.Sprintf("failed to load default token encoder (%s): %s, "+
			"if you are running in an offline environment, set TIKTOKEN_CACHE_DIR to point at pre-downloaded files, "+
			"see https://stackoverflow.com/questions/76106366/how-to-use-tiktoken-in-offline-mode-computer", defaultEncodingName, err.Error()))
	}
}

// encodingNameForModel resolves the tiktoken encoding name for a model WITHOUT
// building the (expensive) BPE core. It mirrors tiktoken.EncodingForModel's
// lookup (exact match, then prefix match) but returns only the name, and falls
// back to the default encoding for unknown models.
func encodingNameForModel(model string) string {
	if name, ok := tiktoken.MODEL_TO_ENCODING[model]; ok {
		return name
	}
	for prefix, name := range tiktoken.MODEL_PREFIX_TO_ENCODING {
		if strings.HasPrefix(model, prefix) {
			return name
		}
	}
	return defaultEncodingName
}

// loadEncoder returns the cached encoder for an encoding name, building and
// caching it on first use. It is safe for concurrent use.
func loadEncoder(name string) (*tiktoken.Tiktoken, error) {
	encoderMu.RLock()
	enc := encoderByName[name]
	encoderMu.RUnlock()
	if enc != nil {
		return enc, nil
	}

	encoderMu.Lock()
	defer encoderMu.Unlock()
	if enc = encoderByName[name]; enc != nil { // re-check after acquiring the write lock
		return enc, nil
	}
	enc, err := tiktoken.GetEncoding(name)
	if err != nil {
		return nil, errors.Wrapf(err, "load tiktoken encoding %q", name)
	}
	encoderByName[name] = enc
	return enc, nil
}

// getTokenEncoder returns a shared encoder for the model. For a healthy process
// it never returns nil (the default encoder is warmed at startup); on an
// unexpected load failure it falls back to the default encoder, and getTokenNum
// degrades to an approximate count if even that is unavailable.
func getTokenEncoder(model string) *tiktoken.Tiktoken {
	name := encodingNameForModel(model)
	enc, err := loadEncoder(name)
	if err == nil {
		return enc
	}
	if name != defaultEncodingName {
		if def, derr := loadEncoder(defaultEncodingName); derr == nil {
			return def
		}
	}
	return nil
}

func getTokenNum(tokenEncoder *tiktoken.Tiktoken, text string) int {
	if config.ApproximateTokenEnabled || tokenEncoder == nil {
		return int(float64(len(text)) * 0.38)
	}
	return len(tokenEncoder.Encode(text, nil, nil))
}

// CountTokenMessages counts the number of tokens in a list of messages.
func CountTokenMessages(ctx context.Context,
	messages []model.Message, actualModel string) int {
	lg := gmw.GetLogger(ctx)

	tokenEncoder := getTokenEncoder(actualModel)
	// Reference:
	// https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
	// https://github.com/pkoukk/tiktoken-go/issues/6
	//
	// Every message follows <|start|>{role/name}\n{content}<|end|>\n
	var tokensPerMessage int
	var tokensPerName int
	if actualModel == "gpt-3.5-turbo-0301" {
		tokensPerMessage = 4
		tokensPerName = -1 // If there's a name, the role is omitted
	} else {
		tokensPerMessage = 3
		tokensPerName = 1
	}

	tokenNum := 0
	var totalAudioTokens float64
	var audioTokensPerSecond float64
	audioTokensConfigured := false
	for _, message := range messages {
		tokenNum += tokensPerMessage
		contents := message.ParseContent()
		for _, content := range contents {
			switch content.Type {
			case model.ContentTypeText:
				if content.Text != nil {
					tokenNum += getTokenNum(tokenEncoder, *content.Text)
				}
			case model.ContentTypeImageURL:
				imageURL := ""
				detail := ""
				if content.ImageURL != nil {
					imageURL = content.ImageURL.Url
					detail = content.ImageURL.Detail
				}
				imageTokens, err := countImageTokens(imageURL, detail, actualModel)
				if err != nil {
					// Provide structured diagnostics without dumping full base64 content
					isDataURL := strings.HasPrefix(imageURL, "data:image/")
					b64Len := 0
					sample := ""
					if isDataURL {
						// Extract after comma
						if idx := strings.Index(imageURL, ","); idx >= 0 && idx+1 < len(imageURL) {
							raw := imageURL[idx+1:]
							b64Len = len(raw)
							if b64Len > 48 {
								sample = raw[:48]
							} else {
								sample = raw
							}
						}
					}
					lg.Error("error counting image tokens",
						zap.Error(err),
						zap.String("model", actualModel),
						zap.Bool("data_url", isDataURL),
						zap.Int("base64_len", b64Len),
						zap.String("detail", detail),
						zap.String("base64_sample", sample),
					)
				} else {
					tokenNum += imageTokens
				}
			case model.ContentTypeInputAudio:
				audioData, err := base64.StdEncoding.DecodeString(content.InputAudio.Data)
				if err != nil {
					lg.Error("error decoding audio data", zap.Error(err))
				}

				if !audioTokensConfigured {
					audioCfg, found := pricing.ResolveAudioPricing(actualModel, nil, &Adaptor{}, time.Time{})
					if found && audioCfg != nil && audioCfg.PromptTokensPerSecond > 0 {
						audioTokensPerSecond = audioCfg.PromptTokensPerSecond
					} else {
						audioTokensPerSecond = pricing.DefaultAudioPromptTokensPerSecond
					}
					audioTokensConfigured = true
				}

				audioTokens, err := helper.GetAudioTokens(ctx,
					bytes.NewReader(audioData),
					audioTokensPerSecond)
				if err != nil {
					lg.Error("error counting audio tokens", zap.Error(err))
				} else {
					totalAudioTokens += audioTokens
				}
			}
		}

		tokenNum += getTokenNum(tokenEncoder, message.Role)
		if message.Name != nil {
			tokenNum += tokensPerName
			tokenNum += getTokenNum(tokenEncoder, *message.Name)
		}
	}
	tokenNum += int(math.Ceil(totalAudioTokens))
	tokenNum += 3 // Every reply is primed with <|start|>assistant<|message|>
	return tokenNum
}

// func countVisonTokenMessages(messages []VisionMessage, model string) (int, error) {
// 	tokenEncoder := getTokenEncoder(model)
// 	// Reference:
// 	// https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
// 	// https://github.com/pkoukk/tiktoken-go/issues/6
// 	//
// 	// Every message follows <|start|>{role/name}\n{content}<|end|>\n
// 	var tokensPerMessage int
// 	var tokensPerName int
// 	if model == "gpt-3.5-turbo-0301" {
// 		tokensPerMessage = 4
// 		tokensPerName = -1 // If there's a name, the role is omitted
// 	} else {
// 		tokensPerMessage = 3
// 		tokensPerName = 1
// 	}
// 	tokenNum := 0
// 	for _, message := range messages {
// 		tokenNum += tokensPerMessage
// 		for _, cnt := range message.Content {
// 			switch cnt.Type {
// 			case OpenaiVisionMessageContentTypeText:
// 				tokenNum += getTokenNum(tokenEncoder, cnt.Text)
// 			case OpenaiVisionMessageContentTypeImageUrl:
// 				imgblob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(cnt.ImageUrl.URL, "data:image/jpeg;base64,"))
// 				if err != nil {
// 					return 0, errors.Wrap(err, "failed to decode base64 image")
// 				}

// 				if imgtoken, err := CountVisionImageToken(imgblob, cnt.ImageUrl.Detail); err != nil {
// 					return 0, errors.Wrap(err, "failed to count vision image token")
// 				} else {
// 					tokenNum += imgtoken
// 				}
// 			}
// 		}

// 		tokenNum += getTokenNum(tokenEncoder, message.Role)
// 		if message.Name != nil {
// 			tokenNum += tokensPerName
// 			tokenNum += getTokenNum(tokenEncoder, *message.Name)
// 		}
// 	}
// 	tokenNum += 3 // Every reply is primed with <|start|>assistant<|message|>
// 	return tokenNum, nil
// }

const (
	// Defaults for 4o/4.1/4.5 family
	lowDetailCost         = 85
	highDetailCostPerTile = 170
	additionalCost        = 85
	// gpt-4o-mini cost higher than other model
	gpt4oMiniLowDetailCost  = 2833
	gpt4oMiniHighDetailCost = 5667
	gpt4oMiniAdditionalCost = 2833
)

// getImageSizeFn is injected for testability
var getImageSizeFn = imgutil.GetImageSize

// getVisionBaseTile returns base and tile tokens for a model family according to docs
func getVisionBaseTile(model string) (base int, tile int) {
	// gpt-4o-mini special case
	if strings.HasPrefix(model, "gpt-4o-mini") {
		return gpt4oMiniAdditionalCost, gpt4oMiniHighDetailCost
	}
	// gpt-5 family (including gpt-5-chat-latest)
	if strings.HasPrefix(model, "gpt-5") {
		return 70, 140
	}
	// o-series (o1, o1-pro, o3)
	if strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") {
		return 75, 150
	}
	// computer-use-preview
	if strings.HasPrefix(model, "computer-use-preview") {
		return 65, 129
	}
	// 4o/4.1/4.5 default family
	if strings.HasPrefix(model, "gpt-4o") || strings.HasPrefix(model, "gpt-4.1") || strings.HasPrefix(model, "gpt-4.5") {
		return additionalCost, highDetailCostPerTile
	}
	// Fallback to 4o/4.1 defaults
	return additionalCost, highDetailCostPerTile
}

func countImageTokens(url string, detail string, model string) (_ int, err error) {
	var fetchSize = true
	var width, height int

	// However, in my test, it seems to be always the same as "high".
	// The following image, which is 125x50, is still treated as high-res, taken
	// 255 tokens in the response of non-stream chat completion api.
	// https://upload.wikimedia.org/wikipedia/commons/1/10/18_Infantry_Division_Messina.jpg
	if detail == "" || detail == "auto" {
		// assume by test, not sure if this is correct
		detail = "high"
	}
	switch detail {
	case "low":
		// Low detail is a flat base token cost per docs
		if strings.HasPrefix(model, "gpt-4o-mini") {
			return gpt4oMiniLowDetailCost, nil
		}
		base, _ := getVisionBaseTile(model)
		return base, nil
	case "high":
		if fetchSize {
			width, height, err = getImageSizeFn(url)
			if err != nil {
				return 0, errors.Wrap(err, "failed to get image size")
			}
		}
		// Claude-specific: cap long edge at 1568 then approx tokens by area/750
		// We detect Claude via model prefix to avoid importing meta here
		if strings.HasPrefix(model, "claude-") ||
			strings.HasPrefix(model, "sonnet") ||
			strings.HasPrefix(model, "haiku") ||
			strings.HasPrefix(model, "opus") {
			// Cap long edge to 1568 while preserving aspect ratio
			maxEdge := 1568.0
			w := float64(width)
			h := float64(height)
			if w > h {
				if w > maxEdge {
					scale := maxEdge / w
					w *= scale
					h *= scale
				}
			} else {
				if h > maxEdge {
					scale := maxEdge / h
					w *= scale
					h *= scale
				}
			}
			tokens := max(int(math.Round((w*h)/750.0)), 0)
			return tokens, nil
		}
		if width > 2048 || height > 2048 { // max(width, height) > 2048
			ratio := float64(2048) / math.Max(float64(width), float64(height))
			width = int(float64(width) * ratio)
			height = int(float64(height) * ratio)
		}
		if width > 768 && height > 768 { // min(width, height) > 768 (scale down to 768 on shortest side)
			ratio := float64(768) / math.Min(float64(width), float64(height))
			width = int(float64(width) * ratio)
			height = int(float64(height) * ratio)
		}
		numSquares := int(math.Ceil(float64(width)/512) * math.Ceil(float64(height)/512))
		if strings.HasPrefix(model, "gpt-4o-mini") {
			return numSquares*gpt4oMiniHighDetailCost + gpt4oMiniAdditionalCost, nil
		}
		base, tile := getVisionBaseTile(model)
		result := numSquares*tile + base
		return result, nil
	default:
		return 0, errors.New("invalid detail option")
	}
}

// CountImageTokens counts token usage for an image URL in vision-capable prompts.
// Parameters: url is the image URL or data URL; detail is "low", "high", or "auto"; model is the target model name.
// Returns: the estimated token count and any error encountered while inspecting the image size.
func CountImageTokens(url string, detail string, model string) (int, error) {
	return countImageTokens(url, detail, model)
}

// CountInputAudioTokens estimates prompt tokens for base64-encoded input audio.
// Parameters: ctx is the request context; base64Data is the audio payload; model is the target model name.
// Returns: the estimated token count for the audio input and any decoding/processing error.
func CountInputAudioTokens(ctx context.Context, base64Data string, model string) (int, error) {
	audioData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return 0, errors.Wrap(err, "decode input audio base64")
	}

	audioTokensPerSecond := pricing.DefaultAudioPromptTokensPerSecond
	audioCfg, found := pricing.ResolveAudioPricing(model, nil, &Adaptor{}, time.Time{})
	if found && audioCfg != nil && audioCfg.PromptTokensPerSecond > 0 {
		audioTokensPerSecond = audioCfg.PromptTokensPerSecond
	}

	audioTokens, err := helper.GetAudioTokens(ctx, bytes.NewReader(audioData), audioTokensPerSecond)
	if err != nil {
		return 0, errors.Wrap(err, "count input audio tokens")
	}

	return int(math.Ceil(audioTokens)), nil
}

func CountTokenInput(input any, model string) int {
	switch v := input.(type) {
	case string:
		return CountTokenText(v, model)
	case []string:
		text := ""
		for _, s := range v {
			text += s
		}
		return CountTokenText(text, model)
	}
	return 0
}

func CountTokenText(text string, model string) int {
	tokenEncoder := getTokenEncoder(model)
	return getTokenNum(tokenEncoder, text)
}

func CountToken(text string) int {
	return CountTokenInput(text, "gpt-3.5-turbo")
}
