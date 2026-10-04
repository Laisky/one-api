package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

const responseUsageJSONPartLimit = 256

// responseUsageJSONPart holds bounded streamed and complete evidence for one JSON content item.
type responseUsageJSONPart struct {
	delta         strings.Builder
	deltaBytes    int
	snapshot      string
	snapshotBytes int
}

// jsonUsageKeys returns bounded index and hashed-ID aliases for one JSON content part.
func (a *ResponseUsageAccumulator) jsonUsageKeys(root map[string]json.RawMessage) (string, string) {
	output, _ := a.counter(root, "output_index")
	content, _ := a.counter(root, "content_index")
	index := "i:" + strconv.Itoa(output) + ":" + strconv.Itoa(content)
	id := responseUsageString(root["item_id"])
	if id == "" {
		return index, ""
	}
	digest := sha256.Sum256([]byte(id))
	alias := "d:" + hex.EncodeToString(digest[:]) + ":" + strconv.Itoa(content)
	if root["output_index"] == nil {
		return alias, ""
	}
	return index, alias
}

// observeJSONText merges per-item deltas and snapshots within fixed text and identity budgets.
func (a *ResponseUsageAccumulator) observeJSONText(root map[string]json.RawMessage, text string, snapshot bool) {
	if text == "" {
		return
	}
	if a.jsonPartKeys == nil {
		a.jsonPartKeys = make(map[string]*responseUsageJSONPart)
	}
	key, alias := a.jsonUsageKeys(root)
	part := a.jsonPartKeys[key]
	if part == nil && alias != "" {
		part = a.jsonPartKeys[alias]
	}
	if part == nil {
		if len(a.jsonParts) == responseUsageJSONPartLimit {
			a.limited = true
			// Beyond the identity budget retain a labelled conservative byte estimate.
			a.jsonOverflowBytes += len(text)
			return
		}
		part = &responseUsageJSONPart{}
		a.jsonParts = append(a.jsonParts, part)
	}
	for _, candidate := range []string{key, alias} {
		if candidate == "" || a.jsonPartKeys[candidate] != nil {
			continue
		}
		if len(a.jsonPartKeys) == 2*responseUsageJSONPartLimit {
			a.limited = true
			continue
		}
		a.jsonPartKeys[candidate] = part
	}
	if snapshot {
		if len(text) <= part.snapshotBytes {
			return
		}
		part.snapshotBytes = len(text)
		if part.snapshotBytes > part.deltaBytes {
			part.snapshot = a.captureJSONPrefix(text)
		}
		return
	}
	part.deltaBytes += len(text)
	part.delta.WriteString(a.captureJSONPrefix(text))
}

// captureJSONPrefix retains at most the shared JSON text budget without splitting UTF-8.
func (a *ResponseUsageAccumulator) captureJSONPrefix(text string) string {
	remaining := responseUsageTextLimit - a.jsonCaptureBytes
	if len(text) > remaining {
		a.limited = true
		text = text[:remaining]
		for len(text) > 0 && !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	a.jsonCaptureBytes += len(text)
	return strings.Clone(text)
}

// completionEvidence combines distinct JSON items after each item's snapshot comparison.
func (a *ResponseUsageAccumulator) completionEvidence() (string, int) {
	text, size := a.text.String(), a.textBytes
	if a.snapshotBytes > size {
		text, size = a.snapshot.String(), a.snapshotBytes
	}
	var combined strings.Builder
	combined.WriteString(text)
	for _, part := range a.jsonParts {
		value, count := part.delta.String(), part.deltaBytes
		if part.snapshotBytes > count {
			value, count = part.snapshot, part.snapshotBytes
		}
		size += count
		remaining := responseUsageTextLimit - combined.Len()
		if len(value) > remaining {
			value = value[:remaining]
			for len(value) > 0 && !utf8.ValidString(value) {
				value = value[:len(value)-1]
			}
		}
		combined.WriteString(value)
	}
	return combined.String(), size + a.jsonOverflowBytes
}

// consumeResponseBlocks supplies output/content indices and item IDs for complete JSON blocks.
func (a *ResponseUsageAccumulator) consumeResponseBlocks(item map[string]json.RawMessage, outputIndex int, snapshot bool) {
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(item["content"], &blocks) != nil {
		return
	}
	for contentIndex, block := range blocks {
		if responseUsageString(block["type"]) != "output_json" {
			a.consumeBlock(block, snapshot)
			continue
		}
		fields := map[string]json.RawMessage{
			"output_index":  json.RawMessage(strconv.Itoa(outputIndex)),
			"content_index": json.RawMessage(strconv.Itoa(contentIndex)),
			"item_id":       item["id"],
		}
		text := responseUsageJSONText(block["json"])
		if text == "" {
			text = responseUsageString(block["text"])
		}
		a.observeJSONText(fields, text, snapshot)
	}
}
