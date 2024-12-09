package replicate

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToFluxRemixRequest(t *testing.T) {
	// Prepare input data
	imageData := []byte{0x89, 0x50, 0x4E, 0x47} // Simulates PNG magic bytes
	maskData := []byte{
		0, 0, 0, 0, // Transparent pixel
		255, 255, 255, 255, // Opaque white pixel
	}
	prompt := "Test prompt"
	model := "Test model"
	responseType := "json"

	request := OpenaiImageEditRequest{
		Image:        imageData,
		Mask:         maskData,
		Prompt:       prompt,
		Model:        model,
		ResponseType: responseType,
	}

	// Call the method under test
	fluxRequest := request.toFluxRemixRequest()

	// Verify FluxInpaintingInput fields
	require.NotNil(t, fluxRequest)
	require.Equal(t, prompt, fluxRequest.Input.Prompt)
	require.Equal(t, 30, fluxRequest.Input.Steps)
	require.Equal(t, 3, fluxRequest.Input.Guidance)
	require.Equal(t, 5, fluxRequest.Input.SafetyTolerance)
	require.False(t, fluxRequest.Input.PromptUnsampling)

	// Check image field (Base64 encoded)
	expectedImageBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageData)
	require.Equal(t, expectedImageBase64, fluxRequest.Input.Image)

	// Check mask field (Base64 encoded and inverted transparency)
	expectedInvertedMask := []byte{
		255, 255, 255, 255, // Transparent pixel inverted to black
		255, 255, 255, 255, // Opaque white pixel remains the same
	}
	expectedMaskBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(expectedInvertedMask)
	require.Equal(t, expectedMaskBase64, fluxRequest.Input.Mask)

	// Verify seed
	// Since the seed is generated based on the current time, we validate its presence
	require.NotZero(t, fluxRequest.Input.Seed)
	require.True(t, fluxRequest.Input.Seed > 0)

	// Additional assertions can be added as necessary
}
