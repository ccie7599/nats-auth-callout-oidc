# Auth-Callout Service

The auth service is a Go application that subscribes to NATS auth-callout requests, validates OIDC tokens against PingOne, maps token scopes to NATS permissions, and returns signed authorization JWTs.

## Overview

```
┌─────────────────────────────────────────────────┐
│  Auth Service (Go)                              │
│                                                 │
│  ┌──────────────┐   ┌────────────────────────┐  │
│  │ OIDC Verifier│   │ Scope Mapper           │  │
│  │              │   │                        │  │
│  │ Discovery    │   │ nats:admin   → pub: >  │  │
│  │ JWKS cache   │   │ nats:publish → pub:    │  │
│  │ Token verify │   │               orders/  │  │
│  │              │   │               events   │  │
│  └──────┬───────┘   │ nats:subscribe → sub:  │  │
│         │           │               orders/  │  │
│         │           │               events   │  │
│         │           └────────┬───────────────┘  │
│         │                    │                  │
│  ┌──────▼────────────────────▼───────────────┐  │
│  │ Authorizer                                │  │
│  │                                           │  │
│  │ 1. Extract token from ConnectOptions      │  │
│  │ 2. Validate against OIDC provider         │  │
│  │ 3. Map scopes to NATS permissions         │  │
│  │ 4. Build + sign UserClaims JWT            │  │
│  │ 5. Publish audit event                    │  │
│  └──────┬────────────────────────────────────┘  │
│         │                                       │
│  ┌──────▼───────┐   ┌────────────────────────┐  │
│  │ NATS Client  │   │ Audit Publisher        │  │
│  │              │   │                        │  │
│  │ subscribe:   │   │ auth.audit.success     │  │
│  │ $SYS.REQ.    │   │ auth.audit.failure     │  │
│  │ USER.AUTH    │   │                        │  │
│  └──────────────┘   └────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

## Source Files

| File | Responsibility |
|---|---|
| `main.go` | Entrypoint — load NKeys, init OIDC verifiers, connect NATS, subscribe to auth-callout |
| `authorizer.go` | Core logic — token extraction, validation, scope mapping, JWT signing |
| `oidc.go` | OIDC provider discovery, JWKS caching, token verification |
| `permissions.go` | Static scope-to-permission mapping |
| `audit.go` | Audit event publisher — success/failure events to `auth.audit.>` |

## Key Code

### Authorizer (authorizer.go)

The authorizer is the core function that processes each auth-callout request:

```go
func NewAuthorizer(verifiers []*OIDCVerifier, signingKey nkeys.KeyPair,
    issuerPubKey string, audit *AuditPublisher) AuthorizerFunc {

    return func(req *jwt.AuthorizationRequestClaims) (string, error) {
        // 1. Extract bearer token from CONNECT options
        rawToken := req.ConnectOptions.Token
        if rawToken == "" {
            rawToken = req.ConnectOptions.Password  // fallback
        }

        if rawToken == "" {
            audit.PublishFailure(AuditEvent{
                Reason: "no authentication token provided",
            })
            return "", fmt.Errorf("no authentication token provided")
        }

        // 2. Validate OIDC token against all configured issuers
        claims, issuer, err := ValidateToken(ctx, rawToken, verifiers)
        if err != nil {
            audit.PublishFailure(AuditEvent{
                Reason: fmt.Sprintf("token validation failed: %v", err),
            })
            return "", fmt.Errorf("authentication failed: %w", err)
        }

        // 3. Map OIDC scopes to NATS permissions
        perms := ResolvePermissions(claims.Scopes)
        if !perms.HasPermissions() {
            return "", fmt.Errorf("no authorized NATS scopes")
        }

        // 4. Build UserClaims JWT
        uc := jwt.NewUserClaims(req.UserNkey)
        uc.Name = claims.Subject
        uc.Audience = "APP"  // target account
        uc.Expires = time.Now().Add(1 * time.Hour).Unix()
        uc.Pub.Allow.Add(perms.PubAllow...)
        uc.Sub.Allow.Add(perms.SubAllow...)
        uc.Resp = &jwt.ResponsePermission{
            MaxMsgs: 1,
            Expires: 5 * time.Minute,
        }

        // 5. Sign and return
        encoded, _ := uc.Encode(signingKey)
        audit.PublishSuccess(AuditEvent{...})
        return encoded, nil
    }
}
```

**Key decisions:**
- Token is extracted from `ConnectOptions.Token` first, with `ConnectOptions.Password` as fallback. This supports both `nats.Token()` and `nats.UserInfo("", token)` client patterns.
- `uc.Audience = "APP"` places the authenticated user in the `APP` account (non-operator mode).
- `uc.Resp` enables request-reply patterns for clients.

### OIDC Verification (oidc.go)

The OIDC verifier uses `coreos/go-oidc/v3` for standard OIDC discovery and token validation:

```go
type OIDCVerifier struct {
    verifier  *oidc.IDTokenVerifier
    provider  *oidc.Provider
    issuerURL string
}

func NewOIDCVerifier(ctx context.Context, issuerURL, audience string) (*OIDCVerifier, error) {
    // Retry loop for startup ordering (OIDC provider may not be ready)
    var provider *oidc.Provider
    for i := 0; i < 15; i++ {
        provider, err = oidc.NewProvider(ctx, issuerURL)
        if err == nil {
            break
        }
        time.Sleep(2 * time.Second)
    }

    config := &oidc.Config{SkipClientIDCheck: true}
    return &OIDCVerifier{
        verifier: provider.Verifier(config),
    }, nil
}
```

**Multi-issuer support**: The auth service accepts comma-separated `OIDC_ISSUER_URL` values and tries each verifier in order during token validation:

```go
func ValidateToken(ctx context.Context, rawToken string,
    verifiers []*OIDCVerifier) (*OIDCClaims, string, error) {
    for _, v := range verifiers {
        claims, err := v.Verify(ctx, rawToken)
        if err == nil {
            return claims, v.issuerURL, nil
        }
    }
    return nil, "", fmt.Errorf("token validation failed against all issuers")
}
```

### Permission Mapping (permissions.go)

Static mapping from OIDC scopes to NATS pub/sub permission lists:

```go
var DefaultScopeMappings = map[string]ScopeMapping{
    "nats:admin": {
        PubAllow: []string{">"},
        SubAllow: []string{">"},
    },
    "nats:publish": {
        PubAllow: []string{"orders.>", "events.>"},
        SubAllow: []string{"_INBOX.>"},
    },
    "nats:subscribe": {
        SubAllow: []string{"orders.>", "events.>", "_INBOX.>"},
    },
}

func ResolvePermissions(scopes []string) *ResolvedPermissions {
    result := &ResolvedPermissions{}
    for _, scope := range scopes {
        mapping, ok := DefaultScopeMappings[scope]
        if !ok {
            continue
        }
        result.PubAllow = append(result.PubAllow, mapping.PubAllow...)
        result.SubAllow = append(result.SubAllow, mapping.SubAllow...)
    }
    return result
}
```

Multiple scopes are merged — a token with both `nats:publish` and `nats:subscribe` would get the union of both permission sets.

### Audit Publisher (audit.go)

Fire-and-forget audit events published to NATS subjects:

```go
type AuditEvent struct {
    Timestamp   time.Time     `json:"timestamp"`
    UserNKey    string        `json:"user_nkey"`
    ClientIP    string        `json:"client_ip"`
    TokenIssuer string        `json:"token_issuer,omitempty"`
    TokenSub    string        `json:"token_sub,omitempty"`
    Scopes      []string      `json:"scopes,omitempty"`
    Decision    string        `json:"decision"`
    Reason      string        `json:"reason,omitempty"`
    Permissions *GrantedPerms `json:"permissions,omitempty"`
}

func (a *AuditPublisher) PublishSuccess(event AuditEvent) {
    event.Decision = "success"
    event.Timestamp = time.Now().UTC()
    a.publish("auth.audit.success", event)
}

func (a *AuditPublisher) PublishFailure(event AuditEvent) {
    event.Decision = "failure"
    event.Timestamp = time.Now().UTC()
    a.publish("auth.audit.failure", event)
}
```

The web dashboard subscribes to `auth.audit.>` to display real-time auth decisions. Events are dropped if no subscriber is connected (core NATS, no JetStream persistence).

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `NATS_URL` | No | `tls://nats:4222` | NATS server URL |
| `NATS_USER` | No | `auth-service` | Username for NATS AUTH account |
| `NATS_PASSWORD` | No | `callout-secret` | Password for NATS AUTH account |
| `OIDC_ISSUER_URL` | **Yes** | — | OIDC issuer URL(s), comma-separated for multi-issuer |
| `OIDC_AUDIENCE` | No | _(skip check)_ | Expected `aud` claim in tokens |
| `NKEY_SEED_FILE` | No | `/nkeys/auth.seed` | Path to NKey private seed file |
| `TLS_CA_FILE` | No | — | CA certificate for NATS TLS |
| `TLS_SERVER_NAME` | No | — | Override TLS server name (for internal Docker traffic) |

## Dependencies

```
github.com/coreos/go-oidc/v3    # OIDC discovery + token verification
github.com/nats-io/jwt/v2       # NATS JWT encoding (UserClaims, AuthorizationResponse)
github.com/nats-io/nats.go      # NATS client
github.com/nats-io/nkeys        # NKey signing
```
