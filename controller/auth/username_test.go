package auth

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultOAuthUsernameShape(t *testing.T) {
	t.Parallel()

	pattern := regexp.MustCompile(`^github_[a-z0-9]{12}$`)
	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		name := defaultOAuthUsername(" GitHub ")
		require.Regexp(t, pattern, name)
		_, dup := seen[name]
		require.False(t, dup, "generated usernames must not repeat: %s", name)
		seen[name] = struct{}{}
	}
}
