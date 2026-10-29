package gemini

import (
	"encoding/base64"
	"github.com/Laisky/errors/v2"
	"io"
	"mime"
	"net/http"
	"strings"
)

// validateNativeImageData validates encoding and recognizable image signatures without exposing input bytes.
// Parameters: declared is the fetched content type and data is the bounded base64 image payload.
// Returns: a suitable content type or a sanitized validation error.
func validateNativeImageData(declared, data string) (string, error) {
	invalid := func() (string, error) { return "", errors.New("validation failed: invalid input image") }
	if data == "" {
		return invalid()
	}
	media, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return invalid()
	}
	media = strings.ToLower(media)
	if media != "application/octet-stream" && !strings.HasPrefix(media, "image/") {
		return invalid()
	}
	if _, err = io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(data))); err != nil {
		return invalid()
	}
	var sample [512]byte
	n, readErr := io.ReadFull(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)), sample[:])
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return invalid()
	}
	detected := http.DetectContentType(sample[:n])
	if media == "application/octet-stream" {
		if strings.HasPrefix(detected, "image/") {
			return detected, nil
		}
		// Keep unknown binary image containers compatible; not all provider-supported formats are sniffed by net/http.
		if detected == "application/octet-stream" {
			return media, nil
		}
		return invalid()
	}
	switch media {
	case "image/jpg":
		media = "image/jpeg"
	}
	switch media {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		if detected != media {
			return invalid()
		}
	}
	return media, nil
}
