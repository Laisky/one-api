package utils

import "slices"

const (
	claudeFable5BedrockModelID   = "anthropic.claude-fable-5"
	claudeFable51BedrockModelID  = "anthropic.claude-fable-5-1"
	claudeMythos51BedrockModelID = "anthropic.claude-mythos-5-1"
)

// init registers the Claude 5.1 global inference profiles using the same source
// Region coverage as Claude Fable 5. It takes no parameters and returns no
// values.
func init() {
	sourceRegions := GlobalProfileSourceRegions[claudeFable5BedrockModelID]
	GlobalProfileSourceRegions[claudeFable51BedrockModelID] = slices.Clone(sourceRegions)
	GlobalProfileSourceRegions[claudeMythos51BedrockModelID] = slices.Clone(sourceRegions)

	CrossRegionInferences = append(
		CrossRegionInferences,
		"global."+claudeFable51BedrockModelID,
		"global."+claudeMythos51BedrockModelID,
	)
}
