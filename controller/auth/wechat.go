package auth

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/model"
)

type wechatLoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// getWeChatIdByCode exchanges a user-supplied WeChat verification code for the
// WeChat id at the configured WeChat server. The code is query-escaped so it
// cannot inject extra parameters into the authenticated verifier request. It
// returns the WeChat id, or an error when the code is empty, rejected or the
// verifier cannot be reached.
func getWeChatIdByCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("Invalid parameter")
	}
	verifierURL := config.WeChatServerAddress + "/api/wechat/user?" + url.Values{"code": {code}}.Encode()
	req, err := http.NewRequest(http.MethodGet, verifierURL, nil)
	if err != nil {
		return "", errors.Wrap(err, "create wechat request")
	}
	req.Header.Set("Authorization", config.WeChatServerToken)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	httpResponse, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "send wechat request")
	}
	defer httpResponse.Body.Close()
	var res wechatLoginResponse
	err = json.NewDecoder(httpResponse.Body).Decode(&res)
	if err != nil {
		return "", errors.Wrap(err, "decode wechat response")
	}
	if !res.Success {
		return "", errors.New(res.Message)
	}
	if res.Data == "" {
		return "", errors.New("Verification code error or expired")
	}
	return res.Data, nil
}

// WeChatAuth logs in or provisions the account linked to the WeChat code in
// the query string, requiring the account's TOTP code through
// controller.CompleteOAuthLogin when 2FA is enabled. It is served on POST
// behind the session mutation guard so a cross-site navigation cannot replace
// an existing dashboard session.
func WeChatAuth(c *gin.Context) {
	ctx := gmw.Ctx(c)
	if !config.WeChatAuthEnabled {
		helper.RespondError(c, errors.New("The administrator has not enabled login and registration via WeChat"))
		return
	}
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user := model.User{
		WeChatId: wechatId,
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		err := user.FillUserByWeChatId()
		if err != nil {
			helper.RespondError(c, err)
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Username = defaultOAuthUsername("wechat")
			user.DisplayName = "WeChat User"
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

// WeChatBind attaches the WeChat id resolved from the query-string code to the
// authenticated account, writing only the wechat_id column. It is served on
// POST behind dashboard authentication.
func WeChatBind(c *gin.Context) {
	if !config.WeChatAuthEnabled {
		helper.RespondError(c, errors.New("The administrator has not enabled login and registration via WeChat"))
		return
	}
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		helper.RespondError(c, errors.New("The WeChat account has been bound"))
		return
	}
	bindOAuthIdentity(c, c.GetInt(ctxkey.Id), model.OAuthIdentityWeChat, wechatId, "")
}
