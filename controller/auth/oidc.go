package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
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

func OidcAuth(c *gin.Context) {
	ctx := gmw.Ctx(c)
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}
	username := session.Get("username")
	if username != nil {
		OidcBind(c)
		return
	}
	if !config.OidcEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Administrator has not enabled OIDC Log in and Sign up",
		})
		return
	}
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		err := user.FillUserByOidcId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Email = oidcUser.Email
			if oidcUser.PreferredUsername != "" {
				user.Username = oidcUser.PreferredUsername
			} else {
				user.Username = "oidc_" + strconv.Itoa(model.GetMaxUserId()+1)
			}
			if oidcUser.Name != "" {
				user.DisplayName = oidcUser.Name
			} else {
				user.DisplayName = "OIDC User"
			}
			err := user.Insert(ctx, 0)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The administrator has turned off new user registration",
			})
			return
		}
	}

	if user.Status != model.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "User has been banned",
			"success": false,
		})
		return
	}
	controller.SetupLogin(&user, c)
}

func OidcBind(c *gin.Context) {
	if !config.OidcEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The administrator has turned off new user registration",
		})
		return
	}
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "This OIDC account has already been bound",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	// id := c.GetInt("id")  // critical bug!
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.OidcId = oidcUser.OpenID
	err = user.Update(false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
	return
}
