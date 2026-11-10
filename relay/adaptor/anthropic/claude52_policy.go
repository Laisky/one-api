package anthropic

// IsClaude52 identifies the two published canonical 5.2 IDs for shared policy.
// It deliberately excludes deployment names, provider namespaces and guessed aliases.
func IsClaude52(name string) bool {
	return name == "claude-fable-5-2" || name == "claude-mythos-5-2"
}
