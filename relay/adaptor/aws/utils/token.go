package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/image"
	netutil "github.com/songquanpeng/one-api/common/network"
	"github.com/songquanpeng/one-api/relay/model"
)

// DownloadImageFromURL downloads an image from a URL and returns the image data and format
// Supports both HTTP/HTTPS URLs and data URIs with base64-encoded images
// This is a public wrapper for the image downloading functionality used by adapters
func DownloadImageFromURL(ctx context.Context, imageURL string) ([]byte, types.ImageFormat, error) {
	return downloadImageFromURL(ctx, imageURL)
}

// downloadImageFromURL downloads an image from a URL and returns the image data and format
// Supports both HTTP/HTTPS URLs and data URIs with base64-encoded images
func downloadImageFromURL(ctx context.Context, imageURL string) ([]byte, types.ImageFormat, error) {
	// Validate URL
	if imageURL == "" {
		return nil, "", errors.New("image URL is empty")
	}

	// Check if this is a data URI (base64-encoded image)
	if strings.HasPrefix(imageURL, "data:") {
		return handleDataURI(imageURL)
	}

	// Handle HTTP/HTTPS URLs (existing logic)
	return downloadImageFromHTTPURL(ctx, imageURL)
}

// handleDataURI processes data URI with base64-encoded image data
// Format: data:image/[format];base64,[base64-encoded-data]
func handleDataURI(dataURI string) ([]byte, types.ImageFormat, error) {
	// Parse data URI format: data:image/format;base64,data
	if !strings.HasPrefix(dataURI, "data:") {
		return nil, "", errors.New("invalid data URI: must start with 'data:'")
	}

	// Find the comma that separates metadata from data
	commaIndex := strings.Index(dataURI, ",")
	if commaIndex == -1 {
		return nil, "", errors.New("invalid data URI: missing comma separator")
	}

	// Extract metadata and data parts
	metadata := dataURI[5:commaIndex] // Skip "data:" prefix
	encodedData := dataURI[commaIndex+1:]

	// Parse metadata: image/format;base64
	if !strings.Contains(metadata, "image/") {
		return nil, "", errors.New("invalid data URI: not an image type")
	}
	if !strings.Contains(metadata, "base64") {
		return nil, "", errors.New("invalid data URI: only base64 encoding supported")
	}

	// Extract image format from metadata
	format, err := detectImageFormatFromDataURI(metadata)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to detect image format from data URI")
	}

	if err := image.ValidateInlineImageBase64Size(encodedData); err != nil {
		return nil, "", errors.Wrap(err, "inline image exceeds size limit")
	}

	// Decode base64 data
	imageData, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to decode base64 image data")
	}

	// Verify we have actual image data
	if len(imageData) == 0 {
		return nil, "", errors.New("decoded image data is empty")
	}

	// Check size limit using configurable MaxInlineImageSizeMB
	maxSizeBytes := image.MaxInlineImageBytes()
	if int64(len(imageData)) > maxSizeBytes {
		return nil, "", errors.Errorf("decoded image data too large: %d bytes (max: %dMB)", len(imageData), config.MaxInlineImageSizeMB)
	}

	// Additional validation: check magic bytes to confirm format
	actualFormat, err := detectImageFormatFromBytes(imageData)
	if err != nil {
		// If we can't detect from bytes, use the format from data URI
		return imageData, format, nil
	}

	// Use the more accurate format detection from bytes if available
	return imageData, actualFormat, nil
}

// downloadImageFromHTTPURL downloads an image from an HTTP/HTTPS URL (original logic)
func downloadImageFromHTTPURL(ctx context.Context, imageURL string) ([]byte, types.ImageFormat, error) {
	// Reject private or local addresses to prevent SSRF.
	if _, err := netutil.ValidateExternalURL(ctx, imageURL); err != nil {
		return nil, "", errors.Wrap(err, "image URL is not allowed")
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to create HTTP request")
	}

	// Set user agent to avoid blocking
	req.Header.Set("User-Agent", "OneAPI-AWS-Image-Downloader/1.0")

	// Make the request
	resp, err := client.UserContentRequestHTTPClient.Do(req)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to download image")
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	format, err := detectImageFormat(contentType, imageURL)
	if err != nil {
		return nil, "", err
	}

	// Read image data with size limit using configurable MaxInlineImageSizeMB
	maxSizeBytes := image.MaxInlineImageBytes()
	imageData, err := io.ReadAll(io.LimitReader(resp.Body, maxSizeBytes+1))
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to read image data")
	}
	if int64(len(imageData)) > maxSizeBytes {
		return nil, "", errors.Errorf("downloaded image data too large: %d bytes (max: %dMB)", len(imageData), config.MaxInlineImageSizeMB)
	}

	// Verify we have actual image data
	if len(imageData) == 0 {
		return nil, "", errors.New("downloaded image is empty")
	}

	// Additional validation: check magic bytes to confirm format
	actualFormat, err := detectImageFormatFromBytes(imageData)
	if err != nil {
		// If we can't detect from bytes, use the content-type/URL detection
		return imageData, format, nil
	}

	// Use the more accurate format detection from bytes if available
	return imageData, actualFormat, nil
}

// detectImageFormatFromDataURI detects image format from data URI metadata
// Format: image/format;base64 -> types.ImageFormat
func detectImageFormatFromDataURI(metadata string) (types.ImageFormat, error) {
	// Convert to lowercase for case-insensitive matching
	lowerMetadata := strings.ToLower(metadata)

	switch {
	case strings.Contains(lowerMetadata, "image/png"):
		return types.ImageFormatPng, nil
	case strings.Contains(lowerMetadata, "image/jpeg"), strings.Contains(lowerMetadata, "image/jpg"):
		return types.ImageFormatJpeg, nil
	case strings.Contains(lowerMetadata, "image/gif"):
		return types.ImageFormatGif, nil
	case strings.Contains(lowerMetadata, "image/webp"):
		return types.ImageFormatWebp, nil
	}

	// If we can't detect the format, return error
	return "", errors.Errorf("unsupported image format in data URI metadata: %s", metadata)
}

// detectImageFormat determines image format from content type and URL
func detectImageFormat(contentType, imageURL string) (types.ImageFormat, error) {
	// First try content type
	switch {
	case strings.Contains(contentType, "image/png"):
		return types.ImageFormatPng, nil
	case strings.Contains(contentType, "image/jpeg"), strings.Contains(contentType, "image/jpg"):
		return types.ImageFormatJpeg, nil
	case strings.Contains(contentType, "image/gif"):
		return types.ImageFormatGif, nil
	case strings.Contains(contentType, "image/webp"):
		return types.ImageFormatWebp, nil
	}

	// Fallback to URL extension
	lowerURL := strings.ToLower(imageURL)
	switch {
	case strings.HasSuffix(lowerURL, ".png"):
		return types.ImageFormatPng, nil
	case strings.HasSuffix(lowerURL, ".jpg"), strings.HasSuffix(lowerURL, ".jpeg"):
		return types.ImageFormatJpeg, nil
	case strings.HasSuffix(lowerURL, ".gif"):
		return types.ImageFormatGif, nil
	case strings.HasSuffix(lowerURL, ".webp"):
		return types.ImageFormatWebp, nil
	}

	// Default to PNG if unable to detect
	return types.ImageFormatPng, nil
}

// detectImageFormatFromBytes detects image format from magic bytes
// Uses proper magic byte validation for robust format detection
func detectImageFormatFromBytes(data []byte) (types.ImageFormat, error) {
	if len(data) < 8 {
		return "", errors.New("insufficient data to detect image format")
	}

	// Check magic bytes with proper validation
	switch {
	case len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A:
		// PNG: Complete 8-byte signature: 89 50 4E 47 0D 0A 1A 0A
		return types.ImageFormatPng, nil

	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		// JPEG: Starts with FF D8, third byte can vary (E0, E1, DB, etc.)
		return types.ImageFormatJpeg, nil

	case len(data) >= 6 &&
		data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 &&
		(data[4] == 0x37 || data[4] == 0x39) && data[5] == 0x61:
		// GIF: GIF87a or GIF89a format validation
		return types.ImageFormatGif, nil

	case len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50:
		// WebP: RIFF header (52 49 46 46) + WEBP signature at bytes 8-11
		return types.ImageFormatWebp, nil
	}

	return "", errors.New("unsupported image format")
}

// CountTokensWithBedrock counts tokens using AWS Bedrock's native CountTokens API
// This provides accurate token counting that matches what would be charged for inference
func CountTokensWithBedrock(ctx context.Context, client *bedrockruntime.Client,
	messages []model.Message, modelID string) (int, error) {
	if client == nil {
		// Fallback to estimation if no client available
		return 0, errors.New("no client available")
	}

	// Convert messages to ConverseTokensRequest format for token counting
	converseTokensRequest, err := convertMessagesToConverseTokensRequest(ctx, messages)
	if err != nil {
		return 0, err
	}

	// Create CountTokensInput using Converse format
	countInput := &bedrockruntime.CountTokensInput{
		ModelId: aws.String(modelID),
		Input: &types.CountTokensInputMemberConverse{
			Value: *converseTokensRequest,
		},
	}

	// Call the CountTokens API
	result, err := client.CountTokens(ctx, countInput)
	if err != nil {
		return 0, errors.Wrap(err, "CountTokens API call failed")
	}

	if result.InputTokens == nil {
		return 0, errors.New("CountTokens returned nil input tokens")
	}

	return int(*result.InputTokens), nil
}

// convertMessagesToConverseTokensRequest converts model.Message slice to ConverseTokensRequest format
func convertMessagesToConverseTokensRequest(ctx context.Context, messages []model.Message) (*types.ConverseTokensRequest, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// System messages go into system field using SystemContentBlockMemberText
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.StringContent(),
			})
		case "user", "assistant":
			// Regular messages
			var contentBlocks []types.ContentBlock

			// Handle different content types
			// AWS Bedrock supports comprehensive content types through the CountTokens API:
			// - ContentBlockMemberText: Text content
			// - ContentBlockMemberImage: Image content (png, jpeg, gif, webp)
			// - ContentBlockMemberDocument: Document content (pdf, csv, doc, docx, xls, xlsx, html, txt, md)
			// - ContentBlockMemberVideo: Video content (mkv, mov, mp4, webm, flv, mpeg, mpg, wmv, three_gp)
			// - ContentBlockMemberToolUse: Tool usage blocks
			// - ContentBlockMemberToolResult: Tool result blocks
			// - ContentBlockMemberGuardContent: Guard content blocks
			// - ContentBlockMemberCachePoint: Cache point blocks
			// - ContentBlockMemberReasoningContent: Reasoning content blocks
			// - ContentBlockMemberCitationsContent: Citations content blocks

			contents := msg.ParseContent()
			if len(contents) == 0 {
				// Simple text content
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.StringContent(),
				})
			} else {
				// Structured content - handle based on current model support
				for _, content := range contents {
					switch content.Type {
					case model.ContentTypeText:
						if content.Text != nil {
							contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
								Value: *content.Text,
							})
						}
					case model.ContentTypeImageURL:
						// Handle image content by downloading actual image data for accurate token counting
						if content.ImageURL != nil {
							// Download actual image data from URL
							imageData, imageFormat, err := downloadImageFromURL(ctx, content.ImageURL.Url)
							if err != nil {
								// If download fails, provide a fallback with error information
								// This maintains functionality while providing debugging information
								return nil, errors.Wrap(err, fmt.Sprintf("failed to download image from URL: %s", content.ImageURL.Url))
							}

							// Create ImageBlock with actual image data
							imageBlock := &types.ContentBlockMemberImage{
								Value: types.ImageBlock{
									Format: imageFormat, // Use detected format from actual image
									Source: &types.ImageSourceMemberBytes{
										Value: imageData, // Use actual downloaded image data
									},
								},
							}
							contentBlocks = append(contentBlocks, imageBlock)
						}
					case model.ContentTypeInputAudio:
						// Handle audio content - AWS Bedrock supports audio in conversations
						if content.InputAudio != nil {
							// For now, use text placeholder as audio tokens are complex to count
							// Future enhancement: Could potentially use ContentBlockMemberDocument
							// or dedicated audio handling when AWS SDK supports it
							contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
								Value: "[AUDIO]", // Placeholder for audio content
							})
						}
					default:
						// For unknown content types, convert to text representation
						// Future enhancement: Add support for document, video, tool use, etc.
						// when model package supports these content types
						contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
							Value: msg.StringContent(),
						})
					}
				}
			}

			converseMsg := types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			}
			converseMessages = append(converseMessages, converseMsg)
		}
	}

	converseTokensRequest := &types.ConverseTokensRequest{
		Messages: converseMessages,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseTokensRequest.System = systemMessages
	}

	return converseTokensRequest, nil
}

// CountTokenMessages counts tokens in messages using AWS Bedrock's CountTokens API
// This is similar to OpenAI's CountTokenMessages but uses AWS native token counting
func CountTokenMessages(ctx context.Context, client *bedrockruntime.Client,
	messages []model.Message, actualModel string) (int, error) {

	// Get AWS model ID
	awsModelID, err := getAWSModelID(actualModel)
	if err != nil {
		return 0, err
	}

	// Use AWS CountTokens API
	tokenCount, err := CountTokensWithBedrock(ctx, client, messages, awsModelID)
	if err != nil {
		return 0, err
	}

	return tokenCount, nil
}

// getAWSModelID converts the request model name to AWS Bedrock model ID
//
// Note: This function may need modification later,
// as some models might be unsupported due to inconsistencies in the AWS Go SDK documentation and its behavior.
func getAWSModelID(requestModel string) (string, error) {
	// Use the existing model ID mapping from Nova adapter
	switch requestModel {
	case "amazon-nova-micro":
		return "uamazon.nova-micro-v1:0", nil
	case "amazon-nova-lite":
		return "amazon.nova-lite-v1:0", nil
	case "amazon-nova-pro":
		return "amazon.nova-pro-v1:0", nil
	case "amazon-nova-premier":
		return "amazon.nova-premier-v1:0", nil
	case "amazon-nova-canvas":
		return "amazon.nova-canvas-v1:0", nil

	// Claude models
	case "claude-instant-1.2":
		return "anthropic.claude-instant-v1", nil
	case "claude-2.0":
		return "anthropic.claude-v2", nil
	case "claude-2.1":
		return "anthropic.claude-v2:1", nil
	case "claude-3-haiku-20240307":
		return "anthropic.claude-3-haiku-20240307-v1:0", nil
	case "claude-3-sonnet-20240229":
		return "anthropic.claude-3-sonnet-20240229-v1:0", nil
	case "claude-3-opus-20240229":
		return "anthropic.claude-3-opus-20240229-v1:0", nil
	case "claude-3-5-sonnet-20240620":
		return "anthropic.claude-3-5-sonnet-20240620-v1:0", nil
	case "claude-3-5-sonnet-20241022":
		return "anthropic.claude-3-5-sonnet-20241022-v2:0", nil
	case "claude-3-5-haiku-20241022":
		return "anthropic.claude-3-5-haiku-20241022-v1:0", nil

	// Llama models
	case "llama3-1-8b-128k":
		return "meta.llama3-1-8b-instruct-v1:0", nil
	case "llama3-1-70b-128k":
		return "meta.llama3-1-70b-instruct-v1:0", nil
	case "llama3-2-1b-131k":
		return "meta.llama3-2-1b-instruct-v1:0", nil
	case "llama3-2-3b-131k":
		return "meta.llama3-2-3b-instruct-v1:0", nil
	case "llama3-2-11b-vision-131k":
		return "meta.llama3-2-11b-instruct-v1:0", nil
	case "llama3-2-90b-128k":
		return "meta.llama3-2-90b-instruct-v1:0", nil
	case "llama3-3-70b-128k":
		return "meta.llama3-3-70b-instruct-v1:0", nil

	// Mistral models
	case "mistral-small-2402":
		return "mistral.mistral-small-2402-v1:0", nil
	case "mistral-large-2402":
		return "mistral.mistral-large-2402-v1:0", nil
	case "mistral-pixtral-large-2502":
		return "mistral.pixtral-large-2502-v1:0", nil

	// Titan models
	case "amazon-titan-text-lite":
		return "amazon.titan-text-lite-v1", nil
	case "amazon-titan-text-express":
		return "amazon.titan-text-express-v1", nil
	case "amazon-titan-text-premier":
		return "amazon.titan-text-premier-v1:0", nil
	case "amazon-titan-embed-text":
		return "amazon.titan-embed-text-v1", nil
	case "amazon-titan-embed-text-v2":
		return "amazon.titan-embed-text-v2:0", nil
	case "amazon-titan-image-generator":
		return "amazon.titan-image-generator-v1", nil

	default:
		return "", errors.Errorf("unsupported model: %s", requestModel)
	}
}

// CountTokenText counts tokens in a text string using AWS Bedrock's CountTokens API
// Similar to OpenAI's CountTokenText but uses AWS native token counting
func CountTokenText(ctx context.Context, client *bedrockruntime.Client, text, modelName string) (int, error) {
	// Create a simple user message for token counting
	messages := []model.Message{
		{
			Role:    "user",
			Content: text,
		},
	}

	return CountTokenMessages(ctx, client, messages, modelName)
}

// GetAccurateTokenCount provides accurate token counting for AWS Bedrock requests
// This is the main function that should be used by all AWS handlers for billing purposes
// Returns an error if the AWS CountTokens API fails, rather than falling back to estimation
func GetAccurateTokenCount(ctx context.Context, client *bedrockruntime.Client,
	messages []model.Message, modelName string) (int, error) {

	// Use AWS native CountTokens API for accurate billing
	tokenCount, err := CountTokenMessages(ctx, client, messages, modelName)
	if err != nil {
		// Return the error instead of falling back to estimation
		return 0, err
	}

	return tokenCount, nil
}
