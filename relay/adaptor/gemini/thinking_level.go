package gemini

import (
	stderrors "errors"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
)

// errThinkingLevelUnsupported rejects a level outside the mapped model's contract.
var errThinkingLevelUnsupported = stderrors.New("reasoning effort is not supported by the selected Gemini model")

// resolveNativeThinkingLevel validates a model-specific level. Empty/default
// uses the supplied model default; an empty default leaves upstream behavior
// unchanged. Callers must select the contract for the mapped upstream model.
func resolveNativeThinkingLevel(requested *string, allowed []string, defaultLevel string) (string, error) {
	level := ""
	if requested != nil {
		level = strings.ToLower(strings.TrimSpace(*requested))
	}
	if level == "" || level == "default" {
		level = defaultLevel
	}
	if level != "" && !slices.Contains(allowed, level) {
		return "", errors.WithStack(errThinkingLevelUnsupported)
	}
	return level, nil
}
