package pubengine

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func (a *App) googleOAuthConfig() *oauth2.Config {
	redirectURL := strings.TrimRight(a.Config.URL, "/") + "/admin/auth/google/callback"
	return &oauth2.Config{
		ClientID:     a.Config.GoogleClientID,
		ClientSecret: a.Config.GoogleClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (a *App) handleGoogleLogin(c echo.Context) error {
	state, err := generateState()
	if err != nil {
		return fmt.Errorf("pubengine: generate oauth state: %w", err)
	}

	sess, _ := session.Get(sessionName, c)
	sess.Values["oauth_state"] = state
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return err
	}

	url := a.googleOAuthConfig().AuthCodeURL(state)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func (a *App) handleGoogleCallback(c echo.Context) error {
	sess, _ := session.Get(sessionName, c)
	expectedState, _ := sess.Values["oauth_state"].(string)
	delete(sess.Values, "oauth_state")
	_ = sess.Save(c.Request(), c.Response())

	if expectedState == "" || c.QueryParam("state") != expectedState {
		return c.Redirect(http.StatusSeeOther, "/admin/?error=invalid_state")
	}

	token, err := a.googleOAuthConfig().Exchange(c.Request().Context(), c.QueryParam("code"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/?error=oauth_failed")
	}

	email, err := fetchGoogleUserEmail(a.googleOAuthConfig().Client(c.Request().Context(), token))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/?error=oauth_failed")
	}

	if !strings.EqualFold(email, a.Config.GoogleAdminEmail) {
		return c.Redirect(http.StatusSeeOther, "/admin/?error=unauthorized_email")
	}

	if err := setAdminSession(c); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/")
}

func fetchGoogleUserEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google userinfo returned %d", resp.StatusCode)
	}

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}
