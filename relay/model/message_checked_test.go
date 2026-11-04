package model

import (
	"encoding/json"
	"testing"
)

// TestCheckedMediaRejectsMalformed checks raw wire shapes before ParseContent
// can discard them. Provider conversion must return these errors to clients.
func TestCheckedMediaRejectsMalformed(t *testing.T) {
	for _, input := range []string{
		`[{"type":"unknown","secret":"do-not-log"}]`,
		`[{"type":"video_url"}]`,
		`[{"type":"video_url","video_url":42}]`,
		`[{"type":"video_url","video_url":{"url":42}}]`,
		`[{"type":"video_url","video_url":{"url":""}}]`,
		`[{"type":"file","file":{"file_id":"one","file_url":"https://example.com/a.pdf"}}]`,
		`[{"type":"file","file":{"file_data":"YQ=="},"file_data":"Yg=="}]`,
		`[{"type":"input_audio","input_audio":{"data":42,"format":"wav"}}]`,
		`[{"type":"image_url","image_url":{}}]`,
		`[{"type":"text","text":42}]`,
		`[null]`,
	} {
		t.Run(input, func(t *testing.T) {
			var content any
			if err := json.Unmarshal([]byte(input), &content); err != nil {
				t.Fatal(err)
			}
			if _, err := (Message{Content: content}).ParseContentChecked(); err == nil {
				t.Fatal("invalid content was silently accepted")
			}
		})
	}
}

// TestCheckedMediaTypedAndWireParity protects the two supported in-memory
// representations used by Chat Completions and protocol bridges.
func TestCheckedMediaTypedAndWireParity(t *testing.T) {
	before, after := "before", "after"
	content := []MessageContent{
		{Type: ContentTypeText, Text: &before},
		{Type: ContentTypeVideoURL, VideoURL: &VideoURL{URL: "https://example.com/video.mp4"}},
		{Type: ContentTypeFile, FileData: "data:application/pdf;base64,YQ=="},
		{Type: ContentTypeText, Text: &after},
	}
	wire, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, input := range []any{content, decoded} {
		parts, err := (Message{Content: input}).ParseContentChecked()
		if err != nil || len(parts) != 4 {
			t.Fatalf("parts = %#v; err = %v", parts, err)
		}
		if parts[1].VideoURL == nil || parts[1].VideoURL.URL != content[1].VideoURL.URL || *parts[3].Text != "after" {
			t.Fatal("media or ordering changed")
		}
	}
}
