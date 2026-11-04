package gemini

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// TestNativeMediaSourceWireFormat checks the actual native JSON, not metadata.
func TestNativeMediaSourceWireFormat(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("fixture-media"))
	for _, tc := range []struct {
		name, kind, mime string
		source           nativeMediaSource
		inline           bool
	}{
		{"inline-video", "video", "video/mp4", nativeMediaSource{URI: "data:video/mp4;base64," + data, Kind: "video"}, true},
		{"public-video", "video", "video/mp4", nativeMediaSource{URI: "https://example.com/clip.mp4?signature=opaque", Kind: "video"}, false},
		{"explicit-video-mime", "video", "video/webm", nativeMediaSource{URI: "https://example.com/download", MIMEType: "video/webm", Kind: "video"}, false},
		{"inline-pdf", "file", "application/pdf", nativeMediaSource{Data: "data:application/pdf;base64," + data}, true},
		{"named-pdf", "file", "application/pdf", nativeMediaSource{Data: data, Filename: "document.pdf"}, true},
		{"public-pdf", "file", "application/pdf", nativeMediaSource{URI: "https://example.com/document.pdf"}, false},
		{"inline-audio", "audio", "audio/mpeg", nativeMediaSource{Data: data, MIMEType: "audio/mp3", Kind: "audio"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part, kind, size, err := prepareNativeMediaSource(tc.source)
			if err != nil || kind != tc.kind {
				t.Fatalf("kind = %q, error = %v", kind, err)
			}
			if tc.inline {
				if part.InlineData == nil || part.FileData != nil || part.InlineData.Data != data || part.InlineData.MimeType != tc.mime || size != 13 {
					t.Fatalf("unexpected inline part: %#v, size %d", part, size)
				}
			} else if part.FileData == nil || part.InlineData != nil || part.FileData.FileURI != tc.source.URI || part.FileData.MimeType != tc.mime || size != 0 {
				t.Fatalf("unexpected reference part: %#v, size %d", part, size)
			}
			wire, err := json.Marshal(part)
			if err != nil || !strings.Contains(string(wire), `"mimeType":"`+tc.mime+`"`) {
				t.Fatalf("invalid native wire payload: %s, error %v", wire, err)
			}
		})
	}
}

// TestNativeMediaSourceRejectsInvalid tests rejection before provider dispatch.
func TestNativeMediaSourceRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source nativeMediaSource
	}{
		{"empty", nativeMediaSource{}},
		{"ambiguous", nativeMediaSource{URI: "https://example.com/a.pdf", Data: "YQ=="}},
		{"unknown-mime", nativeMediaSource{URI: "https://example.com/object"}},
		{"invalid-base64", nativeMediaSource{Data: "private-payload!", MIMEType: "application/pdf"}},
		{"empty-base64", nativeMediaSource{URI: "data:video/mp4;base64,"}},
		{"non-base64-data-uri", nativeMediaSource{URI: "data:application/pdf,private-payload"}},
		{"mime-conflict", nativeMediaSource{URI: "data:audio/wav;base64,YQ==", MIMEType: "video/mp4"}},
		{"disguised-audio", nativeMediaSource{URI: "data:audio/wav;base64,YQ==", Kind: "video"}},
		{"unsupported-mime", nativeMediaSource{Data: "YQ==", MIMEType: "application/octet-stream"}},
		{"local-file", nativeMediaSource{URI: "file:///private.pdf"}},
		{"private-bucket", nativeMediaSource{URI: "gs://private/a.pdf"}},
		{"http", nativeMediaSource{URI: "http://example.com/a.pdf"}},
		{"credentials", nativeMediaSource{URI: "https://secret:password@example.com/a.pdf"}},
		{"fragment", nativeMediaSource{URI: "https://example.com/a.pdf#secret"}},
		{"loopback", nativeMediaSource{URI: "https://127.0.0.1/a.pdf"}},
		{"private-ip", nativeMediaSource{URI: "https://192.168.1.2/a.pdf"}},
		{"metadata-ip", nativeMediaSource{URI: "https://169.254.169.254/a.pdf"}},
		{"localhost", nativeMediaSource{URI: "https://localhost/a.pdf"}},
		{"provider-file", nativeMediaSource{URI: "https://generativelanguage.googleapis.com/v1beta/files/secret", MIMEType: "video/mp4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := prepareNativeMediaSource(tc.source)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-payload") || strings.Contains(err.Error(), "password") {
				t.Fatal("error contains private source data")
			}
		})
	}
}

// TestNativeMediaSourceBound verifies the inline limit before allocation/decoding.
func TestNativeMediaSourceBound(t *testing.T) {
	data := strings.Repeat("A", int((nativeInlineMediaLimit+2)/3*4)+4)
	if _, _, _, err := prepareNativeMediaSource(nativeMediaSource{Data: data, MIMEType: "video/mp4"}); err == nil {
		t.Fatal("oversized inline media accepted")
	}
}
