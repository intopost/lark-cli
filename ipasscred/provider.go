package ipasscred

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/credential"
	"github.com/intopost/lark-cli/envvars"
)

const (
	placeholderUAT = "ipass-managed-uat"
	placeholderTAT = "ipass-managed-tat"
)

type Provider struct{}

func (p *Provider) Name() string  { return "ipass" }
func (p *Provider) Priority() int { return 0 }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	if !proxyModeEnabled() {
		return nil, nil
	}

	appID := os.Getenv(envvars.CliAppID)
	if appID == "" {
		return nil, &credential.BlockError{
			Provider: "ipass",
			Reason:   envvars.AIPowerBaseURL + " is set but " + envvars.CliAppID + " is missing",
		}
	}

	brand := credential.Brand(os.Getenv(envvars.CliBrand))
	if brand == "" {
		brand = credential.BrandFeishu
	}

	acct := &credential.Account{
		AppID:     appID,
		AppSecret: credential.NoAppSecret,
		Brand:     brand,
	}

	switch id := credential.Identity(os.Getenv(envvars.CliDefaultAs)); id {
	case "", credential.IdentityAuto:
		acct.DefaultAs = id
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = id
	default:
		return nil, &credential.BlockError{
			Provider: "ipass",
			Reason:   fmt.Sprintf("invalid %s %q (want user, bot, or auto)", envvars.CliDefaultAs, id),
		}
	}

	switch strictMode := os.Getenv(envvars.CliStrictMode); strictMode {
	case "bot":
		acct.SupportedIdentities = credential.SupportsBot
	case "user":
		acct.SupportedIdentities = credential.SupportsUser
	case "off", "":
		acct.SupportedIdentities = credential.SupportsAll
	default:
		return nil, &credential.BlockError{
			Provider: "ipass",
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envvars.CliStrictMode, strictMode),
		}
	}

	if acct.DefaultAs == "" {
		switch acct.SupportedIdentities {
		case credential.SupportsBot:
			acct.DefaultAs = credential.IdentityBot
		default:
			acct.DefaultAs = credential.IdentityUser
		}
	}

	return acct, nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	if !proxyModeEnabled() {
		return nil, nil
	}

	var value string
	switch req.Type {
	case credential.TokenTypeUAT:
		value = placeholderUAT
	case credential.TokenTypeTAT:
		value = placeholderTAT
	default:
		return nil, nil
	}

	return &credential.Token{
		Value:  value,
		Scopes: "",
		Source: "ipass",
	}, nil
}

func proxyModeEnabled() bool {
	return os.Getenv(envvars.IPassSessionID) != ""
}

func init() {
	credential.Register(&Provider{})
}
