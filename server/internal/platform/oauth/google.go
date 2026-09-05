package oauth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
)

func NewGoogle(clientID, clientSecret, redirectURL string) Provider {
	return newProvider("google", google.Endpoint, []string{"openid", "email", "profile"}, clientID, clientSecret, redirectURL, googleIdentity)
}

// googleIdentity reads the OIDC userinfo endpoint; email_verified comes with
// it, so no second call is needed.
func googleIdentity(ctx context.Context, client *http.Client) (Identity, error) {
	var profile struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}

	if err := getJSON(ctx, client, "https://www.googleapis.com/oauth2/v3/userinfo", &profile); err != nil {
		return Identity{}, fmt.Errorf("fetch userinfo :%w", err)
	}

	return Identity{
		ProviderUID:   profile.Sub,
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		Username:      profile.Name,
	}, nil
}
