package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCVerifier wraps the go-oidc verifier with issuer metadata.
type OIDCVerifier struct {
	verifier  *oidc.IDTokenVerifier
	provider  *oidc.Provider
	issuerURL string
}

// OIDCClaims represents the claims extracted from a validated OIDC token.
type OIDCClaims struct {
	Subject  string   `json:"sub"`
	Email    string   `json:"email"`
	Scope    string   `json:"scope"`
	Scopes   []string `json:"-"`
	ClientID string   `json:"client_id"`
}

// NewOIDCVerifier creates a verifier with retry logic for startup ordering.
func NewOIDCVerifier(ctx context.Context, issuerURL, audience string) (*OIDCVerifier, error) {
	var provider *oidc.Provider
	var err error

	for i := 0; i < 15; i++ {
		provider, err = oidc.NewProvider(ctx, issuerURL)
		if err == nil {
			break
		}
		log.Printf("OIDC provider not ready at %s (attempt %d/15): %v", issuerURL, i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider at %s: %w", issuerURL, err)
	}

	config := &oidc.Config{
		SkipClientIDCheck: true,
	}
	if audience != "" {
		config.SkipClientIDCheck = false
		config.ClientID = audience
	}

	return &OIDCVerifier{
		verifier:  provider.Verifier(config),
		provider:  provider,
		issuerURL: issuerURL,
	}, nil
}

// Verify validates the token and extracts claims.
func (v *OIDCVerifier) Verify(ctx context.Context, rawToken string) (*OIDCClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	if claims.Scope != "" {
		claims.Scopes = strings.Split(claims.Scope, " ")
	}

	// PingOne client_credentials tokens use client_id instead of sub
	if claims.Subject == "" && claims.ClientID != "" {
		claims.Subject = claims.ClientID
	}

	return &claims, nil
}

// ValidateToken tries each verifier in order, returning claims from the first success.
func ValidateToken(ctx context.Context, rawToken string, verifiers []*OIDCVerifier) (*OIDCClaims, string, error) {
	var lastErr error
	for _, v := range verifiers {
		claims, err := v.Verify(ctx, rawToken)
		if err == nil {
			return claims, v.issuerURL, nil
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("token validation failed against all issuers: %w", lastErr)
}
