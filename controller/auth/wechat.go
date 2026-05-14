package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

func getWeChatIdByCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("Invalid parameter")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/wechat/user?code=%s", config.WeChatServerAddress, code), nil)
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
			user.Username = "wechat_" + strconv.Itoa(model.GetMaxUserId()+1)
			user.DisplayName = "WeChat User"
			user.Role = model.RoleCommonUser
			user.Status = model.UserStatusEnabled

			if err := user.Insert(ctx, 0); err != nil {
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
	controller.SetupLogin(&user, c)
}

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
	id := c.GetInt(ctxkey.Id)
	user := model.User{
		Id: id,
	}
	err = user.FillUserById()
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user.WeChatId = wechatId
	err = user.Update(false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}
