package openai

import (
	"fmt"
	"strconv"

	"github.com/Laisky/errors/v2"
)

// MaxResponseAPIFallbackInputItems bounds structural work before normalization
// and conversion, independently of optional conversation persistence. This is a
// gateway safety limit, not a claimed upstream provider API limit.
const MaxResponseAPIFallbackInputItems = 4096

// ValidateResponseAPIFallbackInput rejects an oversized effective fallback turn.
// Call before copying/normalizing caller input and again after state hydration.
// Byte-level request limits remain the responsibility of admission middleware.
func ValidateResponseAPIFallbackInput(input []any) error {
	if len(input) > MaxResponseAPIFallbackInputItems {
		return errors.WithStack(fmt.Errorf("response fallback input exceeds %d items", MaxResponseAPIFallbackInputItems))
	}
	return nil
}

// responseToolCallIDAllocator is scoped to one assistant message, never a user,
// global registry or entire conversation. Used IDs are never removed, so each
// base's next suffix can advance monotonically instead of rescanning prefixes.
// Cost is proportional to input/ID bytes and allocated IDs, not their cube.
type responseToolCallIDAllocator struct {
	used       map[string]struct{}
	nextSuffix map[string]int
}

// reserve registers an ID already present in an assistant message.
func (a *responseToolCallIDAllocator) reserve(id string) {
	if a.used == nil {
		a.used = make(map[string]struct{})
	}
	a.used[id] = struct{}{}
}

// allocate preserves legacy spelling (including the first empty ID) and returns
// a collision-free ID. The existing converter's FIFO map still pairs repeated
// input call IDs with their corresponding emitted IDs and tool results.
func (a *responseToolCallIDAllocator) allocate(id string) string {
	if _, exists := a.used[id]; !exists {
		a.reserve(id)
		return id
	}
	base := id
	if base == "" {
		base = "tool_call"
	}
	suffix := a.nextSuffix[base]
	if suffix < 2 {
		suffix = 2
	}
	for {
		candidate := base + "_" + strconv.Itoa(suffix)
		suffix++
		if _, exists := a.used[candidate]; exists {
			continue
		}
		if a.nextSuffix == nil {
			a.nextSuffix = make(map[string]int)
		}
		a.nextSuffix[base] = suffix
		a.reserve(candidate)
		return candidate
	}
}
