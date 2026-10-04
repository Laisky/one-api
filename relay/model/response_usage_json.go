package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const responseUsageJSONPartLimit = 256
const responseUsageJSONDeltaLimit = 1024

// responseUsageJSONPart holds bounded streamed and complete evidence for one JSON content item.
type responseUsageJSONPart struct {
	deltaBytes    int
	snapshot      string
	snapshotBytes int
}

// responseUsageJSONDelta retains bounded chronological evidence for later item alias reconciliation.
type responseUsageJSONDelta struct {
	part *responseUsageJSONPart
	text string
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
	if alias != "" {
		if other := a.jsonPartKeys[alias]; other != nil {
			if part == nil {
				part = other
			} else if part != other {
				part = a.mergeJSONParts(part, other)
			}
		}
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
	if len(a.jsonDeltas) == responseUsageJSONDeltaLimit {
		a.limited = true
		return
	}
	if prefix := a.captureJSONPrefix(text); prefix != "" {
		a.jsonDeltas = append(a.jsonDeltas, responseUsageJSONDelta{part: part, text: prefix})
	}
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
		if part.snapshotBytes > part.deltaBytes {
			size += part.snapshotBytes
			appendJSONEvidence(&combined, part.snapshot)
		} else {
			size += part.deltaBytes
			for _, delta := range a.jsonDeltas {
				if delta.part == part {
					appendJSONEvidence(&combined, delta.text)
				}
			}
		}
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

// appendJSONEvidence appends one prefix without exceeding the final tokenizer text budget.
func appendJSONEvidence(builder *strings.Builder, text string) {
	remaining := responseUsageTextLimit - builder.Len()
	if len(text) > remaining {
		text = text[:remaining]
		for len(text) > 0 && !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	builder.WriteString(text)
}

// mergeJSONParts reconciles aliases of one item, preserving chronological deltas and counting its snapshot once.
func (a *ResponseUsageAccumulator) mergeJSONParts(first, second *responseUsageJSONPart) *responseUsageJSONPart {
	firstIndex, secondIndex := slices.Index(a.jsonParts, first), slices.Index(a.jsonParts, second)
	if secondIndex < firstIndex {
		first, second = second, first
		firstIndex, secondIndex = secondIndex, firstIndex
	}
	first.deltaBytes += second.deltaBytes
	if second.snapshotBytes > first.snapshotBytes {
		first.snapshot, first.snapshotBytes = second.snapshot, second.snapshotBytes
	}
	for key, part := range a.jsonPartKeys {
		if part == second {
			a.jsonPartKeys[key] = first
		}
	}
	for index := range a.jsonDeltas {
		if a.jsonDeltas[index].part == second {
			a.jsonDeltas[index].part = first
		}
	}
	a.jsonParts = slices.Delete(a.jsonParts, secondIndex, secondIndex+1)
	return first
}
