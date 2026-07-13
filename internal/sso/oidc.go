package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDC struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
}
type Claims struct {
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	EmailVerified     *bool  `json:"email_verified"`
}

func New(ctx context.Context, issuer, clientID, clientSecret, redirect string) (*OIDC, error) {
	if issuer == "" {
		return nil, nil
	}
	if clientID == "" || clientSecret == "" || redirect == "" {
		return nil, errors.New("OIDC client ID, secret, and redirect URL are required when issuer is configured")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return &OIDC{config: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirect, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}, verifier: provider.Verifier(&oidc.Config{ClientID: clientID})}, nil
}
func (o *OIDC) AuthURL(state string) string {
	return o.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}
func (o *OIDC) Exchange(ctx context.Context, code string) (Claims, error) {
	token, err := o.config.Exchange(ctx, code)
	if err != nil {
		return Claims{}, err
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Claims{}, errors.New("OIDC response has no ID token")
	}
	idToken, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err = idToken.Claims(&claims); err != nil {
		return claims, err
	}
	if claims.Email == "" {
		return claims, errors.New("OIDC provider did not return an email address")
	}
	if claims.EmailVerified != nil && !*claims.EmailVerified {
		return claims, errors.New("OIDC email is not verified")
	}
	return claims, nil
}
func State() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
