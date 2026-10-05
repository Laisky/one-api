package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/model"
)

type OidcResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type OidcUser struct {
	OpenID            string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Picture           string `json:"picture"`
}

func getOidcUserInfoByCode(code string) (*OidcUser, error) {
	if code == "" {
		return nil, errors.New("Invalid parameter")
	}
	values := url.Values{}
	values.Set("client_id", config.OidcClientId)
	values.Set("client_secret", config.OidcClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/oidc", config.ServerAddress))
	formData := values.Encode()
	req, err := http.NewRequest("POST", config.OidcTokenEndpoint, strings.NewReader(formData))
	if err != nil {
		return nil, errors.Wrap(err, "build OIDC token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to the OIDC server")
	}
	defer res.Body.Close()
	var oidcResponse OidcResponse
	if err = json.NewDecoder(res.Body).Decode(&oidcResponse); err != nil {
		return nil, errors.Wrap(err, "decode OIDC token response")
	}
	req, err = http.NewRequest("GET", config.OidcUserinfoEndpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build OIDC userinfo request")
	}
	req.Header.Set("Authorization", "Bearer "+oidcResponse.AccessToken)
	res2, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to the OIDC server for user info")
	}
	var oidcUser OidcUser
	if err = json.NewDecoder(res2.Body).Decode(&oidcUser); err != nil {
		return nil, errors.Wrap(err, "decode OIDC user info")
	}
	return &oidcUser, nil
}

// OidcAuth handles the OIDC callback. After the session state check it binds
// the OIDC subject to the signed-in account, or signs in (provisioning when
// registration is open) the linked account, requiring its TOTP code through
// controller.CompleteOAuthLogin when 2FA is enabled.
func OidcAuth(c *gin.Context) {
	ctx := gmw.Ctx(c)
	session := sessions.Default(c)
	if !validateOAuthState(c, "oidc") {
		return
	}
	username := session.Get("username")
	if username != nil {
		OidcBind(c)
		return
	}
	if !config.OidcEnabled {
		helper.RespondError(c, errors.New("Administrator has not enabled OIDC Log in and Sign up"))
		return
	}
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		err := user.FillUserByOidcId()
		if err != nil {
			helper.RespondError(c, err)
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Email = oidcUser.Email
			if oidcUser.PreferredUsername != "" {
				user.Username = oidcUser.PreferredUsername
			} else {
				user.Username = defaultOAuthUsername("oidc")
			}
			if oidcUser.Name != "" {
				user.DisplayName = oidcUser.Name
			} else {
				user.DisplayName = "OIDC User"
			}
			err := user.Insert(ctx, 0)
			if err != nil {
				if controller.IsUsernameAlreadyTakenError(err) {
					controller.RespondUsernameAlreadyExists(c)
					return
				}
				helper.RespondError(c, err)
				return
			}
		} else {
			helper.RespondError(c, errors.New("The administrator has turned off new user registration"))
			return
		}
	}

	if user.Status != model.UserStatusEnabled {
		helper.RespondError(c, errors.New("User has been banned"))
		return
	}
	controller.CompleteOAuthLogin(&user, c)
}

// OidcBind attaches the OIDC subject resolved from the callback code to the
// account of the current dashboard session. The account must pass the
// dashboard checks before the provider is contacted, and only the oidc_id
// column is written. It returns no value and always writes a response.
func OidcBind(c *gin.Context) {
	if !config.OidcEnabled {
		helper.RespondError(c, errors.New("The administrator has turned off new user registration"))
		return
	}
	userID, ok := resolveBindingUserID(c)
	if !ok {
		return
	}
	oidcUser, err := getOidcUserInfoByCode(c.Query("code"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if model.IsOidcIdAlreadyTaken(oidcUser.OpenID) {
		helper.RespondError(c, errors.New("This OIDC account has already been bound"))
		return
	}
	bindOAuthIdentity(c, userID, model.OAuthIdentityOidc, oidcUser.OpenID, "bind")
}
