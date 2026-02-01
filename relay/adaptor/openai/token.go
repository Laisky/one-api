package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/pkoukk/tiktoken-go"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	imgutil "github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// tokenEncoderMap won't grow after initialization
var tokenEncoderMap = map[string]*tiktoken.Tiktoken{}
var defaultTokenEncoder *tiktoken.Tiktoken

func InitTokenEncoders() {
	// Startup-time logging can use global logger; but per request logs below use request-scoped.
	gpt35TokenEncoder, err := tiktoken.EncodingForModel("gpt-3.5-turbo")
	if err != nil {
		panic(fmt.Sprintf("failed to get gpt-3.5-turbo token encoder: %s, "+
			"if you are using in offline environment, please set TIKTOKEN_CACHE_DIR to use exsited files, check this link for more information: https://stackoverflow.com/questions/76106366/how-to-use-tiktoken-in-offline-mode-computer ", err.Error()))
	}
	defaultTokenEncoder = gpt35TokenEncoder
	gpt4oTokenEncoder, err := tiktoken.EncodingForModel("gpt-4o")
	if err != nil {
		panic(fmt.Sprintf("failed to get gpt-4o token encoder: %s", err.Error()))
	}
	gpt4TokenEncoder, err := tiktoken.EncodingForModel("gpt-4")
	if err != nil {
		panic(fmt.Sprintf("failed to get gpt-4 token encoder: %s", err.Error()))
	}
	// Initialize token encoders for OpenAI models using adapter's own pricing
	adaptor := &Adaptor{}
	defaultPricing := adaptor.GetDefaultModelPricing()
	for model := range defaultPricing {
		if strings.HasPrefix(model, "gpt-3.5") {
			tokenEncoderMap[model] = gpt35TokenEncoder
		} else if strings.HasPrefix(model, "gpt-4o") {
			tokenEncoderMap[model] = gpt4oTokenEncoder
		} else if strings.HasPrefix(model, "gpt-4") {
			tokenEncoderMap[model] = gpt4TokenEncoder
		} else {
			tokenEncoderMap[model] = nil
		}
	}
	// token encoders initialized
}

func getTokenEncoder(model string) *tiktoken.Tiktoken {
	tokenEncoder, ok := tokenEncoderMap[model]
	if ok && tokenEncoder != nil {
		return tokenEncoder
	}
	if ok {
		tokenEncoder, err := tiktoken.EncodingForModel(model)
		if err != nil {
			// No request context available here; silently fallback
			tokenEncoder = defaultTokenEncoder
		}
		tokenEncoderMap[model] = tokenEncoder
		return tokenEncoder
	}
	return defaultTokenEncoder
}

func getTokenNum(tokenEncoder *tiktoken.Tiktoken, text string) int {
	if config.ApproximateTokenEnabled {
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
					audioCfg, found := pricing.ResolveAudioPricing(actualModel, nil, &Adaptor{})
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
	audioCfg, found := pricing.ResolveAudioPricing(model, nil, &Adaptor{})
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
