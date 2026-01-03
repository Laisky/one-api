package image_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "golang.org/x/image/webp"

	"github.com/songquanpeng/one-api/common/client"
	img "github.com/songquanpeng/one-api/common/image"
)

type CountingReader struct {
	reader    io.Reader
	BytesRead int
}

func (r *CountingReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.BytesRead += n
	return n, err
}

// retryHTTPGet retries HTTP GET requests with exponential backoff to handle network issues in CI
func retryHTTPGet(url string, maxRetries int) (*http.Response, error) {
	var lastErr error
	for i := range maxRetries {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		lastErr = err
		if i < maxRetries-1 {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second) // exponential backoff
		}
	}
	return nil, lastErr
}

func isNetworkTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "context deadline exceeded")
}

var (
	cases = []struct {
		url    string
		format string
		width  int
		height int
	}{
		{"https://s3.laisky.com/uploads/2025/05/Gfp-wisconsin-madison-the-nature-boardwalk.jpg", "jpeg", 2560, 1669},
		{"https://s3.laisky.com/uploads/2025/05/Basshunter_live_performances.png", "png", 4500, 2592},
		{"https://s3.laisky.com/uploads/2025/05/TO_THE_ONE_SOMETHINGNESS.webp", "webp", 984, 985},
		{"https://s3.laisky.com/uploads/2025/05/01_Das_Sandberg-Modell.gif", "gif", 1917, 1533},
		{"https://s3.laisky.com/uploads/2025/05/102Cervus.jpg", "jpeg", 270, 230},
	}
)

func TestMain(m *testing.M) {
	client.Init()
	m.Run()
}

func TestDecode(t *testing.T) {
	t.Parallel()

	// Bytes read: varies sometimes
	// jpeg: 1063892
	// png: 294462
	// webp: 99529
	// gif: 956153
	// jpeg#01: 32805
	for _, c := range cases {
		t.Run("Decode:"+c.format, func(t *testing.T) {
			t.Parallel()
			t.Logf("testing %s", c.url)
			resp, err := retryHTTPGet(c.url, 3)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode, "status code from %s", c.url)

			reader := &CountingReader{reader: resp.Body}
			img, format, err := image.Decode(reader)
			require.NoErrorf(t, err, "decode image from %s", c.url)
			size := img.Bounds().Size()
			require.Equal(t, c.format, format)
			require.Equal(t, c.width, size.X)
			require.Equal(t, c.height, size.Y)
			t.Logf("Bytes read: %d", reader.BytesRead)
		})
	}

	// Bytes read:
	// jpeg: 4096
	// png: 4096
	// webp: 4096
	// gif: 4096
	// jpeg#01: 4096
	for _, c := range cases {
		t.Run("DecodeConfig:"+c.format, func(t *testing.T) {
			t.Parallel()
			resp, err := retryHTTPGet(c.url, 3)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode, "status code from %s", c.url)

			reader := &CountingReader{reader: resp.Body}
			config, format, err := image.DecodeConfig(reader)
			require.NoError(t, err)
			require.Equal(t, c.format, format)
			require.Equal(t, c.width, config.Width)
			require.Equal(t, c.height, config.Height)
			t.Logf("Bytes read: %d", reader.BytesRead)
		})
	}
}

func TestBase64(t *testing.T) {
	t.Parallel()

	// Bytes read:
	// jpeg: 1063892
	// png: 294462
	// webp: 99072
	// gif: 953856
	// jpeg#01: 32805
	for _, c := range cases {
		t.Run("Decode:"+c.format, func(t *testing.T) {
			t.Parallel()
			resp, err := retryHTTPGet(c.url, 3)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode, "status code from %s", c.url)

			require.Equalf(t, http.StatusOK, resp.StatusCode, "status code from %s", c.url)

			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			encoded := base64.StdEncoding.EncodeToString(data)
			body := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
			reader := &CountingReader{reader: body}
			img, format, err := image.Decode(reader)
			require.NoError(t, err)
			size := img.Bounds().Size()
			require.Equal(t, c.format, format)
			require.Equal(t, c.width, size.X)
			require.Equal(t, c.height, size.Y)
			t.Logf("Bytes read: %d", reader.BytesRead)
		})
	}

	// Bytes read:
	// jpeg: 1536
	// png: 768
	// webp: 768
	// gif: 1536
	// jpeg#01: 3840
	for _, c := range cases {
		t.Run("DecodeConfig:"+c.format, func(t *testing.T) {
			t.Parallel()
			resp, err := retryHTTPGet(c.url, 3)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode, "status code from %s", c.url)

			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			encoded := base64.StdEncoding.EncodeToString(data)
			body := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
			reader := &CountingReader{reader: body}
			config, format, err := image.DecodeConfig(reader)
			require.NoError(t, err)
			require.Equal(t, c.format, format)
			require.Equal(t, c.width, config.Width)
			require.Equal(t, c.height, config.Height)
			t.Logf("Bytes read: %d", reader.BytesRead)
		})
	}
}

func TestValidateDataURLImage(t *testing.T) {
	t.Parallel()
	validDataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	require.NoError(t, img.ValidateDataURLImage(validDataURL))

	invalidPayload := "data:image/png;base64,this-is-not-base64"
	err := img.ValidateDataURLImage(invalidPayload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode data URL image")

	wrongPrefix := "data:text/plain;base64,SGVsbG8gV29ybGQ="
	errp := img.ValidateDataURLImage(wrongPrefix)
	require.Error(t, errp)
	require.Contains(t, errp.Error(), "data:image/")
}

func TestGetImageSize(t *testing.T) {
	t.Parallel()

	for i, c := range cases {
		t.Run("Decode:"+strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			width, height, err := img.GetImageSize(c.url)
			require.NoError(t, err)
			require.Equal(t, c.width, width)
			require.Equal(t, c.height, height)
		})
	}
}

func TestGetImageSizeFromBase64(t *testing.T) {
	t.Parallel()

	// Use embedded minimal images to avoid network flakiness
	// 1x1 transparent PNG
	b64PNG1x1 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9YwVf0sAAAAASUVORK5CYII="
	// 1x1 GIF
	b64GIF1x1 := "R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw=="

	tests := []struct {
		name   string
		b64    string
		width  int
		height int
	}{
		{name: "PNG_1x1", b64: b64PNG1x1, width: 1, height: 1},
		{name: "GIF_1x1", b64: b64GIF1x1, width: 1, height: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			width, height, err := img.GetImageSizeFromBase64(tt.b64)
			require.NoError(t, err)
			require.Equal(t, tt.width, width)
			require.Equal(t, tt.height, height)
		})
	}
}

func TestGetImageFromUrl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantMime   string
		wantErr    bool
		errMessage string
	}{
		{
			name:     "Valid JPEG URL",
			input:    cases[0].url, // Using the existing JPEG test case
			wantMime: "image/jpeg",
			wantErr:  false,
		},
		{
			name:     "Valid PNG URL",
			input:    cases[1].url, // Using the existing PNG test case
			wantMime: "image/png",
			wantErr:  false,
		},
		{
			name:     "Valid Data URL",
			input:    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
			wantMime: "image/png",
			wantErr:  false,
		},
		{
			name:       "Invalid URL",
			input:      "https://invalid.example.com/nonexistent.jpg",
			wantErr:    true,
			errMessage: "failed to fetch image URL",
		},
		{
			name:       "Non-image URL",
			input:      "https://ario.laisky.com/alias/doc",
			wantErr:    true,
			errMessage: "invalid content type",
		},
	}

	for _, tt := range tests {
		// capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mimeType, data, err := img.GetImageFromUrl(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMessage != "" {
					if isNetworkTimeoutError(err) {
						t.Skipf("skipping %s due to network timeout: %v", tt.name, err)
					}
					require.Contains(t, err.Error(), tt.errMessage)
				}
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, data)

			// For data URLs, we should verify the mime type matches the input
			if strings.HasPrefix(tt.input, "data:image/") {
				require.Equal(t, tt.wantMime, mimeType)
				return
			}

			// For regular URLs, verify the base64 data is valid and can be decoded
			decoded, err := base64.StdEncoding.DecodeString(data)
			require.NoError(t, err)
			require.NotEmpty(t, decoded)

			// Verify the decoded data is a valid image
			reader := bytes.NewReader(decoded)
			_, format, err := image.DecodeConfig(reader)
			require.NoError(t, err)
			require.Equal(t, strings.TrimPrefix(tt.wantMime, "image/"), format)
		})
	}
}

func TestGenerateTextImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		text            string
		expectedMime    string
		minWidth        int
		minHeight       int
		expectDefault   bool
		validateContent bool
	}{
		{
			name:            "Basic text",
			text:            "Hello, World!",
			expectedMime:    "image/png",
			minWidth:        200, // Minimum enforced width
			minHeight:       100, // Minimum enforced height
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Empty text should use default",
			text:            "",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   true,
			validateContent: true,
		},
		{
			name:            "Long text with word wrapping",
			text:            "This is a very long text that should demonstrate the word wrapping functionality of the text-to-image generator. The text should automatically wrap to multiple lines for better readability and create a taller image.",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Text with special characters",
			text:            "Special chars: @#$%^&*()_+-={}[]|\\:;\"'<>,.?/~`",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Multiline text with explicit breaks",
			text:            "Line 1: System Status\nLine 2: Connection Error\nLine 3: Retry in 5 seconds",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Error message simulation",
			text:            "Image download failed: connection timeout after 30 seconds",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Single character",
			text:            "X",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
		{
			name:            "Numbers and mixed content",
			text:            "Processing: 1234567890 tokens, 42 images, 3.14159 seconds",
			expectedMime:    "image/png",
			minWidth:        200,
			minHeight:       100,
			expectDefault:   false,
			validateContent: true,
		},
	}

	for _, tt := range tests {
		// capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			imageData, mimeType, err := img.GenerateTextImage(tt.text)

			// Should never error
			require.NoError(t, err)
			require.NotEmpty(t, imageData)
			require.Equal(t, tt.expectedMime, mimeType)

			// Verify the image data is valid PNG
			reader := bytes.NewReader(imageData)
			config, format, err := image.DecodeConfig(reader)
			require.NoError(t, err)
			require.Equal(t, "png", format)

			// Check minimum dimensions
			require.GreaterOrEqual(t, config.Width, tt.minWidth)
			require.GreaterOrEqual(t, config.Height, tt.minHeight)

			// For non-empty text, dimensions should be reasonable based on content
			if !tt.expectDefault && tt.text != "" {
				// Longer text should generally create wider or taller images
				if len(tt.text) > 50 {
					require.Greater(t, config.Width+config.Height, 300)
				}
			}

			if tt.validateContent {
				// Verify we can decode the full image (not just config)
				reader.Reset(imageData)
				decodedImg, _, err := image.Decode(reader)
				require.NoError(t, err)
				require.NotNil(t, decodedImg)

				// Verify image bounds match config
				bounds := decodedImg.Bounds()
				require.Equal(t, config.Width, bounds.Dx())
				require.Equal(t, config.Height, bounds.Dy())
			}

			// Log dimensions for debugging
			t.Logf("Text: %q -> Image: %dx%d (%d bytes)",
				tt.text, config.Width, config.Height, len(imageData))
		})
	}
}

func TestGenerateTextImageBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		text         string
		expectedMime string
	}{
		{
			name:         "Basic text to base64",
			text:         "Hello, Base64!",
			expectedMime: "image/png",
		},
		{
			name:         "Empty text to base64",
			text:         "",
			expectedMime: "image/png",
		},
		{
			name:         "Special characters to base64",
			text:         "Test: @#$%^&*()",
			expectedMime: "image/png",
		},
	}

	for _, tt := range tests {
		// capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base64Data, mimeType, err := img.GenerateTextImageBase64(tt.text)

			// Should never error
			require.NoError(t, err)
			require.NotEmpty(t, base64Data)
			require.Equal(t, tt.expectedMime, mimeType)

			// Verify the base64 data is valid
			decoded, err := base64.StdEncoding.DecodeString(base64Data)
			require.NoError(t, err)
			require.NotEmpty(t, decoded)

			// Verify the decoded data is a valid PNG image
			reader := bytes.NewReader(decoded)
			config, format, err := image.DecodeConfig(reader)
			require.NoError(t, err)
			require.Equal(t, "png", format)
			require.Greater(t, config.Width, 0)
			require.Greater(t, config.Height, 0)

			// Verify consistency with GenerateTextImage
			directImageData, directMimeType, err := img.GenerateTextImage(tt.text)
			require.NoError(t, err)
			require.Equal(t, directMimeType, mimeType)

			// The base64 encoded version should match the direct version
			expectedBase64 := base64.StdEncoding.EncodeToString(directImageData)
			require.Equal(t, expectedBase64, base64Data)

			t.Logf("Text: %q -> Base64 length: %d", tt.text, len(base64Data))
		})
	}
}

func TestGenerateTextImageEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("Very long single word", func(t *testing.T) {
		t.Parallel()

		longWord := strings.Repeat("a", 100)
		imageData, mimeType, err := img.GenerateTextImage(longWord)

		require.NoError(t, err)
		require.NotEmpty(t, imageData)
		require.Equal(t, "image/png", mimeType)

		// Verify image can be decoded
		reader := bytes.NewReader(imageData)
		config, format, err := image.DecodeConfig(reader)
		require.NoError(t, err)
		require.Equal(t, "png", format)
		require.Greater(t, config.Width, 200) // Should be wider than minimum
	})

	t.Run("Text with only whitespace", func(t *testing.T) {
		t.Parallel()

		whitespaceText := "   \t\n   "
		imageData, mimeType, err := img.GenerateTextImage(whitespaceText)

		require.NoError(t, err)
		require.NotEmpty(t, imageData)
		require.Equal(t, "image/png", mimeType)

		// Should still create a valid image
		reader := bytes.NewReader(imageData)
		_, format, err := image.DecodeConfig(reader)
		require.NoError(t, err)
		require.Equal(t, "png", format)
	})

	t.Run("Unicode characters", func(t *testing.T) {
		t.Parallel()

		unicodeText := "Hello 世界 🌍 Unicode text with émojis 🎉"
		imageData, mimeType, err := img.GenerateTextImage(unicodeText)

		require.NoError(t, err)
		require.NotEmpty(t, imageData)
		require.Equal(t, "image/png", mimeType)

		// Verify image can be decoded
		reader := bytes.NewReader(imageData)
		_, format, err := image.DecodeConfig(reader)
		require.NoError(t, err)
		require.Equal(t, "png", format)
	})

	t.Run("Multiple consecutive spaces", func(t *testing.T) {
		t.Parallel()

		spacedText := "Word1     Word2     Word3"
		imageData, mimeType, err := img.GenerateTextImage(spacedText)

		require.NoError(t, err)
		require.NotEmpty(t, imageData)
		require.Equal(t, "image/png", mimeType)

		// Verify image can be decoded
		reader := bytes.NewReader(imageData)
		_, format, err := image.DecodeConfig(reader)
		require.NoError(t, err)
		require.Equal(t, "png", format)
	})
}

func TestGenerateTextImageSizeLimit(t *testing.T) {
	t.Parallel()

	t.Run("Text exceeds maximum image size limit", func(t *testing.T) {
		t.Parallel()

		// Calculate text length that would exceed MaxInlineImageSizeMB (30MB default)
		// Formula: imageWidth * imageHeight * 4 * 1.2 > MaxInlineImageSizeMB * 1024 * 1024
		// With 50 chars per line, charWidth=8, charHeight=16, padding=20:
		// For very long text, we need enough lines to create a large image
		// Approximate calculation: we need ~3000+ lines to exceed 30MB

		// Create text with enough content to exceed the size limit
		// Each line will be ~50 characters, we need many lines
		longText := strings.Repeat(strings.Repeat("A", 50)+"\n", 4000) // 4000 lines of 50 chars each

		imageData, mimeType, err := img.GenerateTextImage(longText)

		// Should return an error about exceeding size limit
		require.Error(t, err)
		require.Empty(t, imageData)
		require.Empty(t, mimeType)

		// Verify the error message contains the expected text
		require.Contains(t, err.Error(), "generated image size would exceed")
		require.Contains(t, err.Error(), "MB limit")
		require.Contains(t, err.Error(), "estimated")
		require.Contains(t, err.Error(), "bytes for text length")

		t.Logf("Expected error received: %v", err)
	})

	t.Run("Text just under size limit should succeed", func(t *testing.T) {
		t.Parallel()

		// Create text that should be large but still within limits
		// Based on the previous test failure, 1000 lines exceeded 30MB (~33.8MB)
		// Let's use 500 lines to stay comfortably under the limit
		mediumText := strings.Repeat(strings.Repeat("B", 50)+"\n", 500) // 500 lines should be under limit

		imageData, mimeType, err := img.GenerateTextImage(mediumText)

		// Should succeed without error
		require.NoError(t, err)
		require.NotEmpty(t, imageData)
		require.Equal(t, "image/png", mimeType)

		// Verify the image can be decoded
		reader := bytes.NewReader(imageData)
		config, format, err := image.DecodeConfig(reader)
		require.NoError(t, err)
		require.Equal(t, "png", format)
		require.Greater(t, config.Width, 200)
		require.Greater(t, config.Height, 100)

		t.Logf("Medium text succeeded: %dx%d image, %d bytes",
			config.Width, config.Height, len(imageData))
	})
}
