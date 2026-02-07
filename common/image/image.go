package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	netutil "github.com/songquanpeng/one-api/common/network"
)

// Regex to match data URL pattern
var dataURLPattern = regexp.MustCompile(`data:image/([^;]+);base64,(.*)`)

// IsImageUrl performs a lightweight check to confirm the URL points to a fetchable image within the configured size limit.
func IsImageUrl(url string) (bool, error) {
	// Enforce public-only URLs to prevent SSRF against internal hosts.
	if _, err := netutil.ValidateExternalURL(context.Background(), url); err != nil {
		return false, errors.Wrap(err, "image URL is not allowed")
	}

	resp, err := client.UserContentRequestHTTPClient.Head(url)
	if err != nil {
		return false, errors.Wrapf(err, "failed to fetch image URL: %s", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// this file may not support HEAD method
		resp, err = client.UserContentRequestHTTPClient.Get(url)
		if err != nil {
			return false, errors.Wrapf(err, "failed to fetch image URL: %s", url)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return false, errors.Errorf("failed to fetch image URL: %s, status code: %d", url, resp.StatusCode)
	}

	// Cap declared size to mitigate DoS via oversized content.
	maxSize := MaxInlineImageBytes()
	if resp.ContentLength > maxSize {
		return false, errors.Errorf("image size should not exceed %dMB: %s, size: %d", config.MaxInlineImageSizeMB, url, resp.ContentLength)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "image/") &&
		!strings.Contains(contentType, "application/octet-stream") {
		return false,
			errors.Errorf("invalid content type: %s, expected image type", contentType)
	}

	return true, nil
}

// GetImageSizeFromUrl downloads the image metadata and returns its width and height in pixels.
func GetImageSizeFromUrl(url string) (width int, height int, err error) {
	isImage, err := IsImageUrl(url)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to fetch image URL")
	}
	if !isImage {
		return 0, 0, errors.New("not an image URL")
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get image from URL")
	}
	defer resp.Body.Close()

	maxSize := MaxInlineImageBytes()
	limited := &io.LimitedReader{R: resp.Body, N: maxSize + 1}
	img, _, err := image.DecodeConfig(limited)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to decode image")
	}
	return img.Width, img.Height, nil
}

// GetImageFromUrl retrieves the image bytes or data URL content, returning the MIME type and base64-encoded payload.
func GetImageFromUrl(url string) (mimeType string, data string, err error) {
	// Check if the URL is a data URL
	matches := dataURLPattern.FindStringSubmatch(url)
	if len(matches) == 3 {
		// URL is a data URL
		if err := ValidateInlineImageBase64Size(matches[2]); err != nil {
			return "", "", errors.Wrap(err, "inline image exceeds size limit")
		}
		mimeType = "image/" + matches[1]
		data = matches[2]
		return
	}

	isImage, err := IsImageUrl(url)
	if err != nil {
		return mimeType, data, errors.Wrap(err, "failed to fetch image URL")
	}
	if !isImage {
		return mimeType, data, errors.New("not an image URL")
	}

	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return mimeType, data, errors.Wrap(err, "failed to get image from URL")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mimeType, data, errors.Errorf("failed to fetch image URL: %s, status code: %d", url, resp.StatusCode)
	}
	maxSize := MaxInlineImageBytes()
	if resp.ContentLength > maxSize {
		return mimeType, data, errors.Errorf("image size should not exceed %dMB: %s, size: %d", config.MaxInlineImageSizeMB, url, resp.ContentLength)
	}

	buffer := bytes.NewBuffer(nil)
	limited := io.LimitReader(resp.Body, maxSize+1)
	_, err = buffer.ReadFrom(limited)
	if err != nil {
		return mimeType, data, errors.Wrap(err, "failed to read image data from response")
	}
	if int64(buffer.Len()) > maxSize {
		return mimeType, data, errors.Errorf("image size should not exceed %dMB: %s", config.MaxInlineImageSizeMB, url)
	}

	mimeType = resp.Header.Get("Content-Type")
	data = base64.StdEncoding.EncodeToString(buffer.Bytes())
	return mimeType, data, nil
}

var (
	reg = regexp.MustCompile(`data:image/([^;]+);base64,`)
)

var readerPool = sync.Pool{
	New: func() any {
		return &bytes.Reader{}
	},
}

// GetImageSizeFromBase64 decodes the base64 image data to determine its dimensions.
func GetImageSizeFromBase64(encoded string) (width int, height int, err error) {
	stripped := reg.ReplaceAllString(encoded, "")
	if err := ValidateInlineImageBase64Size(stripped); err != nil {
		return 0, 0, errors.Wrap(err, "inline image exceeds size limit")
	}
	decoded, err := base64.StdEncoding.DecodeString(stripped)
	if err != nil {
		return 0, 0, err
	}

	reader := readerPool.Get().(*bytes.Reader)
	defer readerPool.Put(reader)
	reader.Reset(decoded)

	img, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0, err
	}

	return img.Width, img.Height, nil
}

// GetImageSize detects whether the input is an inline data URL or remote URL and returns the image dimensions accordingly.
func GetImageSize(image string) (width int, height int, err error) {
	if strings.HasPrefix(image, "data:image/") {
		return GetImageSizeFromBase64(image)
	}
	return GetImageSizeFromUrl(image)
}

// GenerateTextImage renders the supplied text into a PNG image and returns the raw bytes and MIME type.
// This is useful for creating fallback images when image downloads fail, particularly for vision-capable models.
// The function creates a white background with black text, automatically sizing the image based on the text content.
func GenerateTextImage(text string) (imageData []byte, mimeType string, err error) {
	if text == "" {
		text = "Image not available"
	}

	// Calculate image dimensions based on text length
	// Using basic font metrics for sizing
	charWidth := 8.0   // Approximate character width for basic font
	charHeight := 16.0 // Approximate character height for basic font
	padding := 20.0    // Padding around text

	// Calculate text dimensions with word wrapping
	maxCharsPerLine := 50 // Maximum characters per line
	lines := wrapText(text, maxCharsPerLine)

	// Calculate image size
	maxLineWidth := 0
	for _, line := range lines {
		if len(line) > maxLineWidth {
			maxLineWidth = len(line)
		}
	}

	imageWidth := int(math.Ceil(float64(maxLineWidth)*charWidth + padding*2))
	imageHeight := int(math.Ceil(float64(len(lines))*charHeight + padding*2))

	// Ensure minimum size for readability
	if imageWidth < 200 {
		imageWidth = 200
	}
	if imageHeight < 100 {
		imageHeight = 100
	}

	// Check if the estimated image size would exceed the configured limit
	// Estimate PNG size: RGBA (4 bytes per pixel) + PNG overhead (~20% compression ratio)
	estimatedSizeBytes := int64(imageWidth * imageHeight * 4)          // RGBA pixels
	estimatedSizeBytes = estimatedSizeBytes + (estimatedSizeBytes / 5) // Add ~20% for PNG overhead
	maxSizeBytes := MaxInlineImageBytes()

	if estimatedSizeBytes > maxSizeBytes {
		return nil, "", errors.Errorf("generated image size would exceed %dMB limit: estimated %d bytes for text length %d",
			config.MaxInlineImageSizeMB, estimatedSizeBytes, len(text))
	}

	// Create image
	img := image.NewRGBA(image.Rect(0, 0, imageWidth, imageHeight))

	// Fill with white background
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Set up text drawing
	black := color.RGBA{0, 0, 0, 255}
	face := basicfont.Face7x13 // Use basic font
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(black),
		Face: face,
	}

	// Draw each line of text
	lineHeight := int(charHeight)
	startY := int(padding + charHeight)

	for i, line := range lines {
		y := startY + i*lineHeight
		drawer.Dot = fixed.Point26_6{
			X: fixed.Int26_6(int(padding) * 64),
			Y: fixed.Int26_6(y * 64),
		}
		drawer.DrawString(line)
	}

	// Encode to PNG
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to encode image to PNG")
	}

	return buf.Bytes(), "image/png", nil
}

// ValidateDataURLImage ensures the provided data URL has a valid image payload by attempting to decode its metadata.
func ValidateDataURLImage(dataURL string) error {
	if !strings.HasPrefix(dataURL, "data:image/") {
		return errors.New("data URL does not start with data:image/")
	}

	if _, _, err := GetImageSizeFromBase64(dataURL); err != nil {
		return errors.Wrap(err, "decode data URL image")
	}

	return nil
}

// GenerateTextImageBase64 creates a PNG image containing the text and returns its base64 encoding alongside the MIME type.
// This is a convenience wrapper around GenerateTextImage for inline usage.
func GenerateTextImageBase64(text string) (base64Data string, mimeType string, err error) {
	imageData, mimeType, err := GenerateTextImage(text)
	if err != nil {
		return "", "", err
	}

	base64Data = base64.StdEncoding.EncodeToString(imageData)
	return base64Data, mimeType, nil
}

// wrapText breaks long text into multiple lines for better display
func wrapText(text string, maxCharsPerLine int) []string {
	if len(text) <= maxCharsPerLine {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	currentLine := ""

	for _, word := range words {
		// If adding this word would exceed the line length
		if len(currentLine)+len(word)+1 > maxCharsPerLine && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		}

		// Handle very long words by breaking them
		for len(currentLine) > maxCharsPerLine {
			lines = append(lines, currentLine[:maxCharsPerLine])
			currentLine = currentLine[maxCharsPerLine:]
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
