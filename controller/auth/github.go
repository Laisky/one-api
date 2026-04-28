package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/model"
)

type GitHubOAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type GitHubUser struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func getGitHubUserInfoByCode(ctx context.Context, code string) (*GitHubUser, error) {
	if code == "" {
		return nil, errors.New("Invalid parameter")
	}

	logger := gmw.GetLogger(ctx)

	values := map[string]string{"client_id": config.GitHubClientId, "client_secret": config.GitHubClientSecret, "code": code}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, errors.Wrap(err, "marshal GitHub OAuth payload")
	}
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.Wrap(err, "build GitHub OAuth request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to GitHub server")
	}
	defer res.Body.Close()
	var oAuthResponse GitHubOAuthResponse
	if err = json.NewDecoder(res.Body).Decode(&oAuthResponse); err != nil {
		return nil, errors.Wrap(err, "decode GitHub OAuth response")
	}

	// https://docs.github.com/en/rest/users/users?apiVersion=2022-11-28#get-the-authenticated-user
	req, err = http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, errors.Wrap(err, "build GitHub user info request")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oAuthResponse.AccessToken))
	res2, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to GitHub server for user info")
	}
	defer res2.Body.Close()

	githubUserBody, err := io.ReadAll(res2.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read GitHub user info response body")
	}
	logger.Debug("got user info from github", zap.ByteString("payload", githubUserBody))

	var githubUser GitHubUser
	if err = json.Unmarshal(githubUserBody, &githubUser); err != nil {
		return nil, errors.Wrap(err, "decode GitHub user info")
	}
	if githubUser.Login == "" {
		return nil, errors.New("The return value is illegal, the user field is empty, please try again later!")
	}
	return &githubUser, nil
}

func GitHubOAuth(c *gin.Context) {
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
		GitHubBind(c)
		return
	}

	if !config.GitHubOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The administrator did not turn on login and registration via GitHub",
		})
		return
	}
	code := c.Query("code")
	githubUser, err := getGitHubUserInfoByCode(ctx, code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		GitHubId: githubUser.Login,
	}
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		err := user.FillUserByGitHubId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Username = "github_" + strconv.Itoa(model.GetMaxUserId()+1)
			if githubUser.Name != "" {
				user.DisplayName = githubUser.Name
			} else {
				user.DisplayName = "GitHub User"
			}
			user.Email = githubUser.Email
			user.Role = model.RoleCommonUser
			user.Status = model.UserStatusEnabled

			if err := user.Insert(ctx, 0); err != nil {
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

func GitHubBind(c *gin.Context) {
	if !config.GitHubOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The administrator did not turn on login and registration via GitHub",
		})
		return
	}
	code := c.Query("code")
	githubUser, err := getGitHubUserInfoByCode(gmw.Ctx(c), code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		GitHubId: githubUser.Login,
	}
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The GitHub account has been bound",
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
	user.GitHubId = githubUser.Login
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

func GenerateOAuthCode(c *gin.Context) {
	session := sessions.Default(c)
	state := random.GetRandomString(12)
	session.Set("oauth_state", state)
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    state,
	})
}
