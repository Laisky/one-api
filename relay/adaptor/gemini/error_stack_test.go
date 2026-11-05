package gemini

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestNativeSentinelErrorsHaveStacks verifies client messages, causes, and source frames.
func TestNativeSentinelErrorsHaveStacks(t *testing.T) {
	tests := []struct {
		name, message, frame string
		cause                error
		call                 func() error
	}{
		{"video", "video_url requires an object", "convertNativeMediaPart", errNativeVideoURLMissing, func() error {
			_, _, err := convertNativeMediaPart("", model.MessageContent{Type: model.ContentTypeVideoURL})
			return err
		}},
		{"file-id", "file_id requires owner-scoped resolution; use inline file_data or a public file_url", "convertNativeMediaPart", errNativeFileIDUnresolved, func() error {
			_, _, err := convertNativeMediaPart("", model.MessageContent{Type: model.ContentTypeFile, FileID: "id"})
			return err
		}},
		{"audio", "input_audio requires an object", "convertNativeMediaPart", errNativeInputAudioMissing, func() error {
			_, _, err := convertNativeMediaPart("", model.MessageContent{Type: model.ContentTypeInputAudio})
			return err
		}},
		{"audio-format", "unsupported native Gemini input_audio format", "convertNativeMediaPart", errNativeInputAudioFormat, func() error {
			_, _, err := convertNativeMediaPart("", model.MessageContent{Type: model.ContentTypeInputAudio, InputAudio: &model.InputAudio{Format: "bad"}})
			return err
		}},
		{"part", "unsupported native Gemini media part", "convertNativeMediaPart", errNativeMediaPartUnsupported, func() error { _, _, err := convertNativeMediaPart("", model.MessageContent{}); return err }},
		{"modality", "media modality is not supported by the selected Gemini model", "convertNativeMediaPart", errNativeMediaModalityUnsupported, func() error {
			_, _, err := convertNativeMediaPart("unknown", model.MessageContent{Type: model.ContentTypeFile, FileData: "YQ==", MIMEType: "application/pdf"})
			return err
		}},
		{"source-count", "media requires exactly one source", "prepareNativeMediaSource", errMediaSourceCount, func() error { _, _, _, err := prepareNativeMediaSource(nativeMediaSource{}); return err }},
		{"data-uri-base64", "media data URI must contain base64 data", "prepareNativeMediaSource", errMediaDataURINotBase64, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{URI: "data:application/pdf,x"})
			return err
		}},
		{"data-uri-mime", "media data URI requires a valid MIME type", "prepareNativeMediaSource", errMediaDataURIMIMEInvalid, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{URI: "data:;base64,YQ=="})
			return err
		}},
		{"mime-conflict", "media MIME type conflicts with data URI", "prepareNativeMediaSource", errMediaMIMEConflict, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{URI: "data:audio/wav;base64,YQ==", MIMEType: "video/mp4"})
			return err
		}},
		{"mime-unsupported", "unsupported or mismatched media MIME type", "prepareNativeMediaSource", errMediaMIMEUnsupported, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: "YQ==", MIMEType: "text/plain"})
			return err
		}},
		{"inline-bound", "inline media exceeds the relay limit; use a public HTTPS reference", "prepareNativeMediaSource", errInlineMediaTooLargeUseReference, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: strings.Repeat("A", (50_000_000+2)/3*4+4), MIMEType: "application/pdf"})
			return err
		}},
		{"invalid-base64", "media data must be non-empty valid base64", "prepareNativeMediaSource", errMediaDataInvalidBase64, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: "!", MIMEType: "application/pdf"})
			return err
		}},
		{"decoded-bound", "inline media exceeds the relay limit", "prepareNativeMediaSource", errInlineMediaTooLarge, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: strings.Repeat("A", (50_000_000+2)/3*4), MIMEType: "application/pdf"})
			return err
		}},
		{"mime-invalid", "media MIME type is invalid", "normalizeNativeMIME", errMediaMIMEInvalid, func() error {
			_, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: "YQ==", MIMEType: "text/plain; charset=utf-8"})
			return err
		}},
		{"uri-invalid", "media reference must be a public HTTPS URL without user credentials or fragments", "validatePublicMediaURI", errMediaReferenceNotPublicHTTPS, func() error { return validatePublicMediaURI("http://example.com/a.pdf") }},
		{"uri-port", "media reference must use HTTPS port 443", "validatePublicMediaURI", errMediaReferencePort, func() error { return validatePublicMediaURI("https://example.com:444/a.pdf") }},
		{"uri-private", "private media references are not supported", "validatePublicMediaURI", errMediaReferencePrivate, func() error { return validatePublicMediaURI("https://127.0.0.1/a.pdf") }},
		{"uri-provider", "private provider file references require owner-scoped resolution", "validatePublicMediaURI", errMediaReferenceProviderFile, func() error {
			return validatePublicMediaURI("https://generativelanguage.googleapis.com/v1beta/files/id")
		}},
		{"thinking", "reasoning effort is not supported by the selected Gemini model", "resolveNativeThinkingLevel", errThinkingLevelUnsupported, func() error {
			value := "bad"
			_, err := resolveNativeThinkingLevel(&value, []string{"high"}, "")
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			require.Equal(t, tt.message, err.Error())
			require.ErrorIs(t, err, tt.cause)
			require.Contains(t, fmt.Sprintf("%+v", err), tt.frame)
		})
	}
}
