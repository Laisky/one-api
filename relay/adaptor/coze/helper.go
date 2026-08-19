package coze

import (
	"context"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/coze-dev/coze-go"

	"github.com/Laisky/one-api/relay/adaptor/coze/constant/event"
)

func event2StopReason(e *string) string {
	if e == nil || *e == event.Message {
		return ""
	}
	return "stop"
}

func getOAuthToken(config string) (string, error) {
	var oauthConfig coze.OAuthConfig
	err := json.Unmarshal([]byte(config), &oauthConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to load OAuth config")
	}

	oauth, err := coze.LoadOAuthAppFromConfig(&oauthConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to load OAuth config")
	}

	jwtClient, ok := oauth.(*coze.JWTOAuthClient)
	if !ok {
		return "", errors.New("invalid OAuth client type: expected JWT client")
	}

	resp, err := jwtClient.GetAccessToken(context.TODO(), nil)

	if err != nil {
		return "", errors.Wrap(err, "failed to get access token")
	}

	return resp.AccessToken, nil
}
