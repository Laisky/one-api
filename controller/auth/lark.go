package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

type LarkOAuthResponse struct {
	AccessToken string `json:"access_token"`
}

type LarkUser struct {
	Name   string `json:"name"`
	OpenID string `json:"open_id"`
	Email  string `json:"email"`
}

func getLarkUserInfoByCode(code string) (*LarkUser, error) {
	if code == "" {
		return nil, errors.New("Invalid parameter")
	}
	values := map[string]string{
		"client_id":     config.LarkClientId,
		"client_secret": config.LarkClientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  fmt.Sprintf("%s/oauth/lark", config.ServerAddress),
	}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, errors.Wrap(err, "marshal Lark OAuth payload")
	}
	req, err := http.NewRequest("POST", "https://open.feishu.cn/open-apis/authen/v2/oauth/token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.Wrap(err, "build Lark OAuth request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		// Return error without logging - let the caller decide whether to log
		return nil, errors.Wrapf(err, "unable to connect to Lark server")
	}
	defer res.Body.Close()
	var oAuthResponse LarkOAuthResponse
	if err = json.NewDecoder(res.Body).Decode(&oAuthResponse); err != nil {
		return nil, errors.Wrap(err, "decode Lark OAuth response")
	}
	req, err = http.NewRequest("GET", "https://passport.feishu.cn/suite/passport/oauth/userinfo", nil)
	if err != nil {
		return nil, errors.Wrap(err, "build Lark user info request")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oAuthResponse.AccessToken))
	res2, err := client.Do(req)
	if err != nil {
		// Return error without logging - let the caller decide whether to log
		return nil, errors.Wrapf(err, "unable to connect to Lark server for user info")
	}
	var larkUser LarkUser
	if err = json.NewDecoder(res2.Body).Decode(&larkUser); err != nil {
		return nil, errors.Wrap(err, "decode Lark user info")
	}
	return &larkUser, nil
}

// LarkOAuth handles the Lark OAuth callback. After the session state check it
// binds the Lark open id to the signed-in account, or signs in (provisioning
// when registration is open) the linked account, requiring its TOTP code
// through controller.CompleteOAuthLogin when 2FA is enabled.
func LarkOAuth(c *gin.Context) {
	ctx := gmw.Ctx(c)
	session := sessions.Default(c)
	if !validateOAuthState(c, "lark") {
		return
	}
	username := session.Get("username")
	if username != nil {
		LarkBind(c)
		return
	}
	code := c.Query("code")
	larkUser, err := getLarkUserInfoByCode(code)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user := model.User{
		LarkId: larkUser.OpenID,
	}
	if model.IsLarkIdAlreadyTaken(user.LarkId) {
		err := user.FillUserByLarkId()
		if err != nil {
			helper.RespondError(c, err)
			return
		}
	} else {
		if config.RegisterEnabled {
			parts := strings.Split(larkUser.Email, "@")
			if len(parts) > 1 {
				user.Username = parts[0]
			} else {
				user.Username = defaultOAuthUsername("lark")
			}
			if larkUser.Name != "" {
				user.DisplayName = larkUser.Name
			} else {
				user.DisplayName = "Lark User"
			}
			user.Role = model.RoleCommonUser
			user.Status = model.UserStatusEnabled

			if err := user.Insert(ctx, 0); err != nil {
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

// LarkBind attaches the Lark open id resolved from the callback code to the
// account of the current dashboard session. The account must pass the
// dashboard checks before Lark is contacted, and only the lark_id column is
// written. It returns no value and always writes a response.
func LarkBind(c *gin.Context) {
	userID, ok := resolveBindingUserID(c)
	if !ok {
		return
	}
	larkUser, err := getLarkUserInfoByCode(c.Query("code"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if model.IsLarkIdAlreadyTaken(larkUser.OpenID) {
		helper.RespondError(c, errors.New("This Lark account has already been bound"))
		return
	}
	bindOAuthIdentity(c, userID, model.OAuthIdentityLark, larkUser.OpenID, "bind")
}
