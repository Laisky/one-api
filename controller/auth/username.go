package auth

import (
	"strings"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// defaultUsernameRandomLength is the length of the random suffix appended to
// generated OAuth usernames.
const defaultUsernameRandomLength = 12

// defaultUsernameAttempts bounds the collision-retry loop; with a 12-character
// alphanumeric suffix a collision is astronomically unlikely, so this only
// guards against a pathological RNG.
const defaultUsernameAttempts = 5

// defaultOAuthUsername mints a username for a freshly registered OAuth user as
// "<source>_<random12>". It deliberately avoids the historical
// "<source>_<GetMaxUserId()+1>" form, which leaked the total user count and the
// account's internal integer id through a public field.
// Parameters:
//   - source: identity provider slug (e.g. "github", "oidc", "lark", "wechat").
//
// Return values:
//   - string: a username that was not taken at the time of the check.
func defaultOAuthUsername(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	candidate := ""
	for i := 0; i < defaultUsernameAttempts; i++ {
		candidate = source + "_" + strings.ToLower(random.GetRandomString(defaultUsernameRandomLength))
		if model.DB == nil || !model.IsUsernameAlreadyTaken(candidate) {
			return candidate
		}
	}
	return candidate
}
