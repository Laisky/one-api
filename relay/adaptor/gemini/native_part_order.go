package gemini

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// nativePartKey identifies one generated part by its source message field and offset.
type nativePartKey struct {
	tool  bool
	index int
}

// nativeContentReference returns an internal source reference for a content part at index.
func nativeContentReference(index int) model.NativePartReference {
	return model.NativePartReference{ContentIndex: &index}
}

// nativeToolReference returns an internal source reference for a function call at index.
func nativeToolReference(index int) model.NativePartReference {
	return model.NativePartReference{ToolCallIndex: &index}
}

// nativeReferenceKey validates a reference and returns its unique source key or an error.
func nativeReferenceKey(reference model.NativePartReference) (nativePartKey, error) {
	if (reference.ContentIndex == nil) == (reference.ToolCallIndex == nil) {
		return nativePartKey{}, errors.New("native part reference must identify exactly one source")
	}
	key := nativePartKey{}
	if reference.ToolCallIndex != nil {
		key.tool = true
		key.index = *reference.ToolCallIndex
	} else {
		key.index = *reference.ContentIndex
	}
	if key.index < 0 {
		return nativePartKey{}, errors.New("native part reference index must be nonnegative")
	}
	return key, nil
}

// reorderNativeParts restores a lowered message's original part boundaries and order.
// Parameters: parts and sources describe generated parts; order names each source exactly once.
// Returns a reordered copy or an error if conversion omitted, duplicated, or cannot identify a part.
func reorderNativeParts(parts []Part, sources, order []model.NativePartReference) ([]Part, error) {
	if len(parts) != len(sources) || len(parts) != len(order) {
		return nil, errors.New("native part order does not cover generated parts")
	}
	positions := make(map[nativePartKey]int, len(sources))
	for i, reference := range sources {
		key, err := nativeReferenceKey(reference)
		if err != nil {
			return nil, errors.Wrap(err, "generated native part source")
		}
		if _, duplicate := positions[key]; duplicate {
			return nil, errors.New("native part source is duplicated")
		}
		positions[key] = i
	}
	out := make([]Part, 0, len(parts))
	for _, reference := range order {
		key, err := nativeReferenceKey(reference)
		if err != nil {
			return nil, errors.Wrap(err, "requested native part source")
		}
		index, exists := positions[key]
		if !exists {
			return nil, errors.New("native part order references a missing or duplicate source")
		}
		out = append(out, parts[index])
		delete(positions, key)
	}
	return out, nil
}

// nativeGatewayPosition copies a supplied gateway position for internal native conversion.
func nativeGatewayPosition(extra *model.ToolExtraContent) *int {
	index, present := model.GeminiPartIndex(extra)
	if !present {
		return nil
	}
	return &index
}

// hasNativeGatewayPositions reports whether any part requests explicit positional restoration.
func hasNativeGatewayPositions(parts []Part) bool {
	for _, part := range parts {
		if part.GatewayPartIndex != nil {
			return true
		}
	}
	return false
}

// restoreNativePartOrder validates complete public positions or applies internal source references.
func restoreNativePartOrder(parts []Part, sources, order []model.NativePartReference) ([]Part, error) {
	if !hasNativeGatewayPositions(parts) {
		return reorderNativeParts(parts, sources, order)
	}
	out := make([]Part, len(parts))
	seen := make([]bool, len(parts))
	for _, part := range parts {
		if part.GatewayPartIndex == nil {
			return nil, errors.New("native part positions must cover every part")
		}
		index := *part.GatewayPartIndex
		if index < 0 || index >= len(parts) || seen[index] {
			return nil, errors.New("native part positions must be a contiguous unique permutation")
		}
		out[index] = part
		seen[index] = true
	}
	return out, nil
}
