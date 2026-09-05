package oauth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

type Identity struct {
	ProviderUID   string
	Email         string
	EmailVerified bool
	Username      string
}

type Provider interface {
	Name() string
	AuthCodeURL(state string) string
	Identity(ctx context.Context, code string) (Identity, error)
}

// provider is the shape every OAuth login shares: exchange the code, then ask
// the vendor who the user is with the resulting client. Only identity differs.
type provider struct {
	name     string
	cfg      *oauth2.Config
	identity func(ctx context.Context, client *http.Client) (Identity, error)
}

func newProvider(name string, endpoint oauth2.Endpoint, scopes []string, clientID, clientSecret, redirectURL string, identity func(context.Context, *http.Client) (Identity, error)) Provider {
	return &provider{
		name: name,
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     endpoint,
			Scopes:       scopes,
		},
		identity: identity,
	}
}

func (p *provider) Name() string { return p.name }

func (p *provider) AuthCodeURL(state string) string { return p.cfg.AuthCodeURL(state) }

func (p *provider) Identity(ctx context.Context, code string) (Identity, error) {
	tok, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("exchange code :%w", err)
	}
	return p.identity(ctx, p.cfg.Client(ctx, tok))
}
