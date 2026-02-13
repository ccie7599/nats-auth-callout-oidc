# Auth-Callout Flow

## Overview

NATS [auth-callout](https://docs.nats.io/running-a-nats-service/configuration/auth_callout) delegates authentication decisions to an external service. Instead of managing credentials directly, the NATS server forwards connection requests to the auth service, which validates the client's OIDC token and returns a signed JWT with the appropriate permissions.

## Sequence Diagram

```
┌────────┐         ┌────────────┐         ┌──────────────┐         ┌──────────┐
│ Client │         │ NATS Server│         │ Auth Service  │         │ PingOne  │
└───┬────┘         └─────┬──────┘         └──────┬───────┘         └────┬─────┘
    │                    │                       │                      │
    │  1. Fetch token    │                       │                      │
    │  (client_creds)    │                       │                      │
    │──────────────────────────────────────────────────────────────────▶│
    │                    │                       │                      │
    │  2. Access token   │                       │                      │
    │◀──────────────────────────────────────────────────────────────────│
    │                    │                       │                      │
    │  3. CONNECT        │                       │                      │
    │  (auth_token=JWT)  │                       │                      │
    │───────────────────▶│                       │                      │
    │                    │                       │                      │
    │                    │  4. $SYS.REQ.USER.AUTH│                      │
    │                    │  (AuthorizationRequest│                      │
    │                    │   Claims JWT)         │                      │
    │                    │──────────────────────▶│                      │
    │                    │                       │                      │
    │                    │                       │  5. Fetch JWKS       │
    │                    │                       │  (cached after first)│
    │                    │                       │─────────────────────▶│
    │                    │                       │                      │
    │                    │                       │  JWKS response       │
    │                    │                       │◀─────────────────────│
    │                    │                       │                      │
    │                    │                       │  6. Validate JWT     │
    │                    │                       │  signature + claims  │
    │                    │                       │                      │
    │                    │                       │  7. Map scopes to    │
    │                    │                       │  NATS permissions    │
    │                    │                       │                      │
    │                    │                       │  8. Build + sign     │
    │                    │                       │  UserClaims JWT      │
    │                    │                       │                      │
    │                    │  9. AuthorizationResp │                      │
    │                    │  (signed UserClaims)  │                      │
    │                    │◀─────────────────────│                      │
    │                    │                       │                      │
    │                    │  10. Publish audit    │                      │
    │                    │  event (auth.audit.>) │                      │
    │                    │◀─────────────────────│                      │
    │                    │                       │                      │
    │  11. +OK / -ERR    │                       │                      │
    │◀──────────────────│                       │                      │
    │                    │                       │                      │
    │  12. Pub/Sub       │                       │                      │
    │  (permissions      │                       │                      │
    │   enforced)        │                       │                      │
    │◀──────────────────▶│                       │                      │
```

## Step-by-Step

### Step 1-2: Token Acquisition

The client obtains an access token from PingOne using the **client_credentials** grant type. Each PingOne application is configured with specific scopes that determine what the client can do in NATS.

```bash
curl -X POST https://auth.pingone.com/<env-id>/as/token \
  -u "<client_id>:<client_secret>" \
  -d "grant_type=client_credentials" \
  -d "scope=nats:publish"
```

Response:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### Step 3: NATS Connection

The client connects to NATS and passes the access token in the `auth_token` field of the CONNECT protocol message:

**Go client:**
```go
nc, err := nats.Connect("tls://nats:4222", nats.Token(accessToken))
```

**JavaScript (WebSocket):**
```javascript
const nc = await connect({
  servers: "wss://nats-demo.example.com:8443",
  token: accessToken,
});
```

### Step 4: Auth-Callout Dispatch

The NATS server does **not** authenticate the client itself. Instead, it publishes the connection request to `$SYS.REQ.USER.AUTH` as an `AuthorizationRequestClaims` JWT. This JWT includes:

- `ConnectOptions.Token` — the client's bearer token
- `ClientInformation.Host` — client IP address
- `UserNkey` — a temporary NKey assigned by the server
- `Server.ID` — the NATS server's ID

Only the auth service (authenticated to the `AUTH` account with username/password) can subscribe to this subject.

### Step 5-6: OIDC Token Validation

The auth service validates the client's access token using standard OIDC:

1. **Discovery**: Fetch `/.well-known/openid-configuration` from the issuer URL to find the JWKS endpoint
2. **JWKS**: Fetch the JSON Web Key Set containing the issuer's public keys
3. **Verification**: Validate the JWT signature, expiry, and issuer claim against the JWKS

```go
provider, _ := oidc.NewProvider(ctx, "https://auth.pingone.com/<env-id>/as")
verifier := provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
idToken, err := verifier.Verify(ctx, rawToken)
```

The JWKS is cached after the first fetch — subsequent validations don't require network calls to PingOne.

### Step 7: Scope-to-Permission Mapping

The auth service extracts scopes from the validated token and maps them to NATS permissions:

```go
var scopeMappings = map[string]ScopeMapping{
    "nats:admin":     {PubAllow: []string{">"}, SubAllow: []string{">"}},
    "nats:publish":   {PubAllow: []string{"orders.>", "events.>"}, SubAllow: []string{"_INBOX.>"}},
    "nats:subscribe": {SubAllow: []string{"orders.>", "events.>", "_INBOX.>"}},
}
```

If the token contains no recognized NATS scopes, the auth service returns an error and the connection is rejected.

### Step 8-9: UserClaims JWT

The auth service builds a `UserClaims` JWT with the resolved permissions and signs it with the shared NKey:

```go
uc := jwt.NewUserClaims(req.UserNkey)
uc.Name = claims.Subject
uc.Audience = "APP"                              // target account
uc.Expires = time.Now().Add(1 * time.Hour).Unix()
uc.Pub.Allow.Add(perms.PubAllow...)
uc.Sub.Allow.Add(perms.SubAllow...)

response := jwt.NewAuthorizationResponseClaims(req.UserNkey)
response.Audience = req.Server.ID
response.Jwt = uc.Encode(signingKey)              // NKey-signed
```

The signed response is published back to the NATS server via the request reply subject.

### Step 10: Audit Trail

After each authorization decision, the auth service publishes a JSON audit event:

- **Success**: `auth.audit.success` — includes granted permissions, scopes, client IP
- **Failure**: `auth.audit.failure` — includes rejection reason, client IP

These events are fire-and-forget (core NATS, no JetStream). The web dashboard subscribes to `auth.audit.>` to display real-time auth decisions.

### Step 11-12: Permission Enforcement

The NATS server either accepts the connection (`+OK`) with the granted permissions, or rejects it (`-ERR`). Once connected, any publish or subscribe attempt that violates the granted permissions is rejected by the NATS server at the protocol level.

## Rejection Flow

When authentication fails (invalid token, expired token, no NATS scopes), the flow short-circuits:

```
Steps 1-4:  Same as above
Step 5-6:   Token validation FAILS
Step 7:     Auth service returns AuthorizationResponseClaims with Error field set
Step 8:     NATS server sends -ERR to client
Step 9:     Connection closed
```

The auth service publishes an `auth.audit.failure` event with the rejection reason before returning the error response.

## NKey Trust Model

The auth-callout protocol uses NKeys for trust between the NATS server and auth service:

```
┌─────────────────┐              ┌─────────────────┐
│  NATS Server    │              │  Auth Service    │
│                 │              │                  │
│  auth_callout { │    trusts    │  signingKey =    │
│    issuer: APUB │◄────────────▶│    FromSeed(seed)│
│  }              │              │                  │
│                 │              │  pubKey = APUB   │
└─────────────────┘              └─────────────────┘
         ▲                                ▲
         │         ┌───────────┐          │
         └─────────│  NKey     │──────────┘
                   │  Volume   │
                   │           │
                   │ auth.pub  │ ← public key (NATS server reads)
                   │ auth.seed │ ← private seed (auth service reads)
                   └───────────┘
```

The NKey pair is generated at startup by an init container and shared via a Docker volume. The NATS server trusts the public key (`issuer`), and the auth service signs responses with the private seed. This means only the auth service can produce valid authorization responses.
