package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestReview607ParseMedia uses the existing public parser contract. It fails on
// the reviewed revision for video, nested file, and interleaved-media payloads.
func TestReview607ParseMedia(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          []string
	}{
		{"text", `"hello"`, []string{"text"}},
		{"image", `[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]`, []string{"image_url"}},
		{"audio", `[{"type":"input_audio","input_audio":{"data":"YQ==","format":"wav"}}]`, []string{"input_audio"}},
		{"flat-file", `[{"type":"file","file_data":"data:application/pdf;base64,YQ=="}]`, []string{"file"}},
		{"video", `[{"type":"video_url","video_url":{"url":"data:video/mp4;base64,YQ=="}}]`, []string{"video_url"}},
		{"nested-file", `[{"type":"file","file":{"file_data":"data:application/pdf;base64,YQ=="}}]`, []string{"file"}},
		{"ordered-media", `[{"type":"text","text":"before"},{"type":"video_url","video_url":{"url":"https://example.com/a.mp4"}},{"type":"text","text":"after"}]`, []string{"text", "video_url", "text"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content any
			if err := json.Unmarshal([]byte(tc.content), &content); err != nil {
				t.Fatal(err)
			}
			parsed := (Message{Content: content}).ParseContent()
			var got []string
			for _, part := range parsed {
				got = append(got, part.Type)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsed types = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestReview607LegacyFileParsing keeps the old permissive decoder contract for
// unrelated providers. Native Gemini's checked parser rejects this ambiguity.
func TestReview607LegacyFileParsing(t *testing.T) {
	parts := (Message{Content: []any{map[string]any{
		"type": "file", "file_id": "legacy-id", "file_data": "legacy-data",
	}}}).ParseContent()
	if len(parts) != 1 || parts[0].FileID != "legacy-id" || parts[0].FileData != "legacy-data" {
		t.Fatal("legacy permissive flat-file parsing changed")
	}
}
