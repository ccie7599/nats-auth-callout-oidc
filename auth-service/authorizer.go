package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// AuthorizerFunc validates an auth request and returns a signed UserClaims JWT.
type AuthorizerFunc func(req *jwt.AuthorizationRequestClaims) (string, error)

// NewAuthorizer returns an AuthorizerFunc that validates OIDC tokens and maps scopes to NATS permissions.
func NewAuthorizer(verifiers []*OIDCVerifier, signingKey nkeys.KeyPair, issuerPubKey string, audit *AuditPublisher) AuthorizerFunc {
	return func(req *jwt.AuthorizationRequestClaims) (string, error) {
		rawToken := req.ConnectOptions.Token
		if rawToken == "" {
			rawToken = req.ConnectOptions.Password
		}

		clientIP := req.ClientInformation.Host

		if rawToken == "" {
			audit.PublishFailure(AuditEvent{
				UserNKey: req.UserNkey,
				ClientIP: clientIP,
				Reason:   "no authentication token provided",
			})
			return "", fmt.Errorf("no authentication token provided")
		}

		// Validate OIDC token against all configured issuers
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		claims, issuer, err := ValidateToken(ctx, rawToken, verifiers)
		if err != nil {
			audit.PublishFailure(AuditEvent{
				UserNKey: req.UserNkey,
				ClientIP: clientIP,
				Reason:   fmt.Sprintf("token validation failed: %v", err),
			})
			return "", fmt.Errorf("authentication failed: %w", err)
		}

		log.Printf("Token validated: sub=%s scopes=%v issuer=%s", claims.Subject, claims.Scopes, issuer)

		// Map OIDC scopes to NATS permissions
		perms := ResolvePermissions(claims.Scopes)
		if !perms.HasPermissions() {
			audit.PublishFailure(AuditEvent{
				UserNKey:    req.UserNkey,
				ClientIP:    clientIP,
				TokenIssuer: issuer,
				TokenSub:    claims.Subject,
				Scopes:      claims.Scopes,
				Reason:      "no authorized NATS scopes in token",
			})
			return "", fmt.Errorf("no authorized NATS scopes found for subject %s (scopes: %v)", claims.Subject, claims.Scopes)
		}

		// Build UserClaims JWT
		uc := jwt.NewUserClaims(req.UserNkey)
		uc.Name = claims.Subject
		uc.Audience = "APP"
		uc.Expires = time.Now().Add(1 * time.Hour).Unix()
		uc.IssuedAt = time.Now().Unix()

		uc.Pub.Allow.Add(perms.PubAllow...)
		uc.Sub.Allow.Add(perms.SubAllow...)

		// Allow request-reply
		uc.Resp = &jwt.ResponsePermission{
			MaxMsgs: 1,
			Expires: 5 * time.Minute,
		}

		encoded, err := uc.Encode(signingKey)
		if err != nil {
			return "", fmt.Errorf("failed to sign user claims: %w", err)
		}

		audit.PublishSuccess(AuditEvent{
			UserNKey:    req.UserNkey,
			ClientIP:    clientIP,
			TokenIssuer: issuer,
			TokenSub:    claims.Subject,
			Scopes:      claims.Scopes,
			Permissions: &GrantedPerms{
				PubAllow: perms.PubAllow,
				SubAllow: perms.SubAllow,
			},
		})

		log.Printf("Authorized %s (sub=%s) pub=%v sub=%v", req.UserNkey, claims.Subject, perms.PubAllow, perms.SubAllow)
		return encoded, nil
	}
}
