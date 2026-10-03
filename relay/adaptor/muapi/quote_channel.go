package muapi

import (
	"crypto/subtle"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// muAPIQuoteDestination binds pricing to distribution's selected channel rather
// than the request metadata's URL or credential. It returns a validated endpoint
// and configured key only after identity, route and credential agreement. No
// extra database query or network operation is needed for this consistency gate.
func muAPIQuoteDestination(c *gin.Context, info *meta.Meta, modelName string) (string, string, error) {
	if c == nil || info == nil {
		return "", "", errors.New("MuAPI quote requires selected channel context")
	}
	value, ok := c.Get(ctxkey.ChannelModel)
	channel, typed := value.(*dbmodel.Channel)
	if !ok || !typed || channel == nil || channel.Id <= 0 || channel.Id != info.ChannelId ||
		channel.UUID != info.ChannelUUID || channel.Type != channeltype.MuAPI || info.ChannelType != channel.Type {
		return "", "", errors.New("MuAPI quote channel identity does not match selected channel")
	}
	base := channel.GetBaseURL()
	if base == "" {
		base = channeltype.ChannelBaseURLs[channeltype.MuAPI]
	}
	// Match the persisted-task resolver's route fence. Do not accept a different
	// proxy/account path just because both URLs share the same authority.
	if strings.TrimRight(base, "/") != strings.TrimRight(info.BaseURL, "/") ||
		subtle.ConstantTimeCompare([]byte(channel.Key), []byte(info.APIKey)) != 1 {
		return "", "", errors.New("MuAPI quote route or credential does not match selected channel")
	}
	endpoint, err := muAPIEndpointURL(base, "models", modelName, "estimate-cost")
	if err != nil {
		return "", "", errors.Wrap(err, "validate configured MuAPI quote destination")
	}
	return endpoint, channel.Key, nil
}
