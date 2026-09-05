package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/oauth2/github"
)

func NewGithub(clientID, clientSecret, redirectURL string) Provider {
	return newProvider("github", github.Endpoint, []string{"read:user", "user:email"}, clientID, clientSecret, redirectURL, githubIdentity)
}

func githubIdentity(ctx context.Context, client *http.Client) (Identity, error) {
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}

	if err := getJSON(ctx, client, "https://api.github.com/user", &profile); err != nil {
		return Identity{}, fmt.Errorf("fetch user :%w", err)
	}

	var email []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := getJSON(ctx, client, "https://api.github.com/user/emails", &email); err != nil {
		return Identity{}, fmt.Errorf("fetch email :%w", err)
	}

	id := Identity{
		ProviderUID: strconv.FormatInt(profile.ID, 10),
		Username:    profile.Login,
	}

	for _, e := range email {
		if e.Primary && e.Verified {
			id.Email = e.Email
			id.EmailVerified = true
			break
		}
	}

	return id, nil
}

func getJSON(ctx context.Context, c *http.Client, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
