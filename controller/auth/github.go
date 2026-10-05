package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
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

// GitHubOAuth handles the GitHub OAuth callback. After the session state check
// it binds the GitHub login to the signed-in account, or signs in (provisioning
// when registration is open) the linked account, requiring its TOTP code
// through controller.CompleteOAuthLogin when 2FA is enabled.
func GitHubOAuth(c *gin.Context) {
	ctx := gmw.Ctx(c)
	session := sessions.Default(c)
	if !validateOAuthState(c, "github") {
		return
	}
	username := session.Get("username")
	if username != nil {
		GitHubBind(c)
		return
	}

	if !config.GitHubOAuthEnabled {
		helper.RespondError(c, errors.New("The administrator did not turn on login and registration via GitHub"))
		return
	}
	code := c.Query("code")
	githubUser, err := getGitHubUserInfoByCode(ctx, code)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user := model.User{
		GitHubId: githubUser.Login,
	}
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		err := user.FillUserByGitHubId()
		if err != nil {
			helper.RespondError(c, err)
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Username = defaultOAuthUsername("github")
			if githubUser.Name != "" {
				user.DisplayName = githubUser.Name
			} else {
				user.DisplayName = "GitHub User"
			}
			user.Email = githubUser.Email
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

// GitHubBind attaches the GitHub login resolved from the callback code to the
// account of the current dashboard session. The account must pass the
// dashboard checks before GitHub is contacted, and only the github_id column
// is written. It returns no value and always writes a response.
func GitHubBind(c *gin.Context) {
	if !config.GitHubOAuthEnabled {
		helper.RespondError(c, errors.New("The administrator did not turn on login and registration via GitHub"))
		return
	}
	userID, ok := resolveBindingUserID(c)
	if !ok {
		return
	}
	githubUser, err := getGitHubUserInfoByCode(gmw.Ctx(c), c.Query("code"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if model.IsGitHubIdAlreadyTaken(githubUser.Login) {
		helper.RespondError(c, errors.New("The GitHub account has been bound"))
		return
	}
	bindOAuthIdentity(c, userID, model.OAuthIdentityGitHub, githubUser.Login, "bind")
}

// GenerateOAuthCode issues a fresh OAuth state, stores it in the session and
// returns it. The route is guarded against cross-site navigations so a hostile
// page cannot replace the state of an in-progress login.
func GenerateOAuthCode(c *gin.Context) {
	session := sessions.Default(c)
	state := random.GetRandomString(12)
	session.Set("oauth_state", state)
	err := session.Save()
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    state,
	})
}
