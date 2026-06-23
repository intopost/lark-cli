package ipasscred

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"lark-cli-ipass/envvars"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
		}
	})
}

func TestResolveAccount_NotActive(t *testing.T) {
	unsetEnv(t, envvars.IPassSessionID)

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct != nil {
		t.Fatal("expected nil account when proxy mode is disabled")
	}
}

func TestResolveAccount_Active(t *testing.T) {
	setEnv(t, envvars.IPassSessionID, "sess_123")
	setEnv(t, envvars.CliAppID, "managed-by-ipass")
	setEnv(t, envvars.CliBrand, "lark")
	unsetEnv(t, envvars.CliDefaultAs)
	unsetEnv(t, envvars.CliStrictMode)

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct == nil {
		t.Fatal("expected non-nil account")
	}
	if acct.AppID != "managed-by-ipass" {
		t.Fatalf("AppID = %q, want managed-by-ipass", acct.AppID)
	}
	if acct.Brand != credential.BrandLark {
		t.Fatalf("Brand = %q, want %q", acct.Brand, credential.BrandLark)
	}
	if acct.AppSecret != credential.NoAppSecret {
		t.Fatalf("AppSecret = %q, want empty", acct.AppSecret)
	}
	if acct.DefaultAs != credential.IdentityUser {
		t.Fatalf("DefaultAs = %q, want %q", acct.DefaultAs, credential.IdentityUser)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Fatalf("SupportedIdentities = %d, want %d", acct.SupportedIdentities, credential.SupportsAll)
	}
}

func TestResolveAccount_MissingAppID(t *testing.T) {
	setEnv(t, envvars.IPassSessionID, "sess_123")
	unsetEnv(t, envvars.CliAppID)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
}

func TestResolveAccount_InvalidModes(t *testing.T) {
	setEnv(t, envvars.IPassSessionID, "sess_123")
	setEnv(t, envvars.CliAppID, "managed-by-ipass")
	setEnv(t, envvars.CliDefaultAs, "admin")

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
}

func TestResolveToken_Placeholders(t *testing.T) {
	setEnv(t, envvars.IPassSessionID, "sess_123")

	p := &Provider{}

	uat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("resolve UAT: %v", err)
	}
	if uat == nil || uat.Value != placeholderUAT {
		t.Fatalf("UAT = %#v, want %q", uat, placeholderUAT)
	}

	tat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatalf("resolve TAT: %v", err)
	}
	if tat == nil || tat.Value != placeholderTAT {
		t.Fatalf("TAT = %#v, want %q", tat, placeholderTAT)
	}
}
