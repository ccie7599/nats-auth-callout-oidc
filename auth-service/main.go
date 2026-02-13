package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func main() {
	natsURL := envOrDefault("NATS_URL", "tls://nats:4222")
	authUser := envOrDefault("NATS_USER", "auth-service")
	authPass := envOrDefault("NATS_PASSWORD", "callout-secret")
	seedFile := envOrDefault("NKEY_SEED_FILE", "/nkeys/auth.seed")
	issuerURLs := mustEnv("OIDC_ISSUER_URL")
	oidcAudience := os.Getenv("OIDC_AUDIENCE")
	tlsCAFile := os.Getenv("TLS_CA_FILE")
	tlsServerName := os.Getenv("TLS_SERVER_NAME")

	// Load account signing key
	seedBytes, err := os.ReadFile(seedFile)
	if err != nil {
		log.Fatalf("Failed to read NKey seed file %s: %v", seedFile, err)
	}
	signingKey, err := nkeys.FromSeed(seedBytes)
	if err != nil {
		log.Fatalf("Failed to parse NKey seed: %v", err)
	}
	pubKey, _ := signingKey.PublicKey()
	log.Printf("Loaded signing key: %s", pubKey)

	// Initialize OIDC verifiers (multi-issuer)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var verifiers []*OIDCVerifier
	for _, issuerURL := range strings.Split(issuerURLs, ",") {
		issuerURL = strings.TrimSpace(issuerURL)
		if issuerURL == "" {
			continue
		}
		log.Printf("Initializing OIDC verifier for issuer: %s", issuerURL)
		v, err := NewOIDCVerifier(ctx, issuerURL, oidcAudience)
		if err != nil {
			log.Printf("WARNING: Failed to initialize OIDC verifier for %s: %v (will skip)", issuerURL, err)
			continue
		}
		verifiers = append(verifiers, v)
		log.Printf("OIDC verifier ready for: %s", issuerURL)
	}
	if len(verifiers) == 0 {
		log.Fatal("No OIDC verifiers could be initialized")
	}

	// Connect to NATS as auth-service user
	opts := []nats.Option{
		nats.UserInfo(authUser, authPass),
		nats.Name("oidc-auth-callout"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	if tlsCAFile != "" {
		caCert, err := os.ReadFile(tlsCAFile)
		if err != nil {
			log.Fatalf("Failed to read TLS CA file %s: %v", tlsCAFile, err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsCfg := &tls.Config{
			RootCAs: caCertPool,
		}
		if tlsServerName != "" {
			tlsCfg.ServerName = tlsServerName
		}
		opts = append(opts, nats.Secure(tlsCfg))
	}

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer nc.Close()
	log.Printf("Connected to NATS at %s", natsURL)

	// Create audit publisher
	audit := NewAuditPublisher(nc)

	// Build authorizer function
	authorizerFn := NewAuthorizer(verifiers, signingKey, pubKey, audit)

	// Subscribe to auth callout requests
	sub, err := nc.Subscribe("$SYS.REQ.USER.AUTH", func(msg *nats.Msg) {
		handleAuthRequest(msg, authorizerFn, signingKey, pubKey)
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to auth callout: %v", err)
	}
	defer sub.Unsubscribe()

	log.Println("Auth callout service started, waiting for authorization requests...")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down auth callout service")
}

func handleAuthRequest(msg *nats.Msg, authorizerFn AuthorizerFunc, signingKey nkeys.KeyPair, issuerPubKey string) {
	// Decode the authorization request
	reqClaims, err := jwt.DecodeAuthorizationRequestClaims(string(msg.Data))
	if err != nil {
		log.Printf("Failed to decode auth request: %v", err)
		respondWithError(msg, signingKey, issuerPubKey, "", "", fmt.Sprintf("invalid request: %v", err))
		return
	}

	userNKey := reqClaims.UserNkey
	serverID := reqClaims.Server.ID
	log.Printf("Auth request for user NKey: %s (client: %s, name: %s, server: %s)",
		userNKey, reqClaims.ClientInformation.Host, reqClaims.ConnectOptions.Name, serverID)

	// Run the authorizer
	userJWT, err := authorizerFn(reqClaims)
	if err != nil {
		log.Printf("Authorization denied for %s: %v", userNKey, err)
		respondWithError(msg, signingKey, issuerPubKey, userNKey, serverID, err.Error())
		return
	}

	// Build success response
	rc := jwt.NewAuthorizationResponseClaims(userNKey)
	rc.Audience = reqClaims.Server.ID
	rc.Jwt = userJWT

	token, err := rc.Encode(signingKey)
	if err != nil {
		log.Printf("Failed to encode response: %v", err)
		respondWithError(msg, signingKey, issuerPubKey, userNKey, serverID, "internal error")
		return
	}

	if err := msg.Respond([]byte(token)); err != nil {
		log.Printf("Failed to respond: %v", err)
	}
}

func respondWithError(msg *nats.Msg, signingKey nkeys.KeyPair, issuerPubKey, userNKey, serverID, errMsg string) {
	if userNKey == "" {
		userNKey = "UNKNOWN"
	}
	rc := jwt.NewAuthorizationResponseClaims(userNKey)
	rc.Audience = serverID
	rc.Error = errMsg

	token, err := rc.Encode(signingKey)
	if err != nil {
		log.Printf("Failed to encode error response: %v", err)
		return
	}
	if err := msg.Respond([]byte(token)); err != nil {
		log.Printf("Failed to respond with error: %v", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return v
}
