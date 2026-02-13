# PingOne Setup Guide

This guide walks through configuring PingOne as the OIDC identity provider for NATS auth-callout. You'll create a custom resource with NATS-specific scopes and three OIDC applications with distinct permissions.

## Prerequisites

- A PingOne account ([free trial](https://www.pingidentity.com/en/try-ping.html))
- An environment created in PingOne (the trial creates one automatically)

## Step 1: Note Your Environment ID

1. Log into the PingOne admin console
2. Navigate to **Settings** → **Environment** → **Properties**
3. Copy the **Environment ID** — you'll need this for the issuer URL

Your OIDC issuer URL will be:
```
https://auth.pingone.com/<environment-id>/as
```

## Step 2: Create a Custom Resource

PingOne custom resources let you define your own OAuth scopes. We'll create one for NATS with three scopes.

1. Navigate to **Applications** → **Resources**
2. Click **+ Add Resource** → **Custom Resource**
3. Set the name to `nats-api`
4. Click **Save**

### Add Scopes

With the `nats-api` resource selected:

1. Go to the **Scopes** tab
2. Add these three scopes:

| Scope Name | Description |
|---|---|
| `nats:admin` | Full NATS access — publish and subscribe to all subjects |
| `nats:publish` | Publish to order and event subjects |
| `nats:subscribe` | Subscribe to order and event subjects |

## Step 3: Create OIDC Applications

Create three OIDC Web Applications, each representing a different access level. All three use the **client_credentials** grant type (machine-to-machine, no user login required).

### 3a: Admin Application

1. Navigate to **Applications** → **Applications**
2. Click **+ Add Application**
3. Choose **OIDC Web App**
4. Set the name to `nats-admin`
5. Click **Save**

**Configure the application:**

1. Go to the **Configuration** tab
2. Under **Grant Types**, ensure **Client Credentials** is selected
3. Note the **Client ID** and **Client Secret**

**Assign scope:**

1. Go to the **Resources** tab
2. Click **+ Add Resource**
3. Select `nats-api`
4. Check only `nats:admin`
5. Click **Save**

**Enable the application:**

1. Toggle the application to **Enabled** (top-right switch)

### 3b: Publisher Application

Repeat the same steps with:
- Name: `nats-publisher`
- Scope: `nats:publish` (from `nats-api` resource)

### 3c: Subscriber Application

Repeat the same steps with:
- Name: `nats-subscriber`
- Scope: `nats:subscribe` (from `nats-api` resource)

## Step 4: Configure Environment Variables

Copy `.env.example` to `.env` and fill in the credentials:

```bash
# PingOne Issuer URL
PING_ISSUER_URL=https://auth.pingone.com/<environment-id>/as

# Admin app (nats:admin scope)
PING_CLIENT_ID=<admin-client-id>
PING_CLIENT_SECRET=<admin-client-secret>

# Publisher app (nats:publish scope)
PING_PUB_CLIENT_ID=<publisher-client-id>
PING_PUB_CLIENT_SECRET=<publisher-client-secret>

# Subscriber app (nats:subscribe scope)
PING_SUB_CLIENT_ID=<subscriber-client-id>
PING_SUB_CLIENT_SECRET=<subscriber-client-secret>

# Auth service password (shared between NATS server and auth-service)
AUTH_SERVICE_PASSWORD=callout-secret
```

## Step 5: Configure Nginx Token Proxy

The web dashboard fetches tokens from PingOne through an Nginx reverse proxy. This keeps client secrets server-side (never exposed to the browser).

For each PingOne application, generate a Base64-encoded Basic Auth header:

```bash
# Admin
echo -n "<admin-client-id>:<admin-client-secret>" | base64

# Publisher
echo -n "<publisher-client-id>:<publisher-client-secret>" | base64

# Subscriber
echo -n "<subscriber-client-id>:<subscriber-client-secret>" | base64
```

Update `nginx/nginx.conf` with the encoded values in the `proxy_set_header Authorization` lines:

```nginx
# Admin app
location /api/token/admin {
    proxy_pass https://auth.pingone.com/<env-id>/as/token;
    proxy_set_header Host auth.pingone.com;
    proxy_set_header Authorization "Basic <base64-encoded-admin-creds>";
    proxy_set_header Content-Type $http_content_type;
    proxy_ssl_server_name on;
}

# Publisher app
location /api/token/publisher {
    proxy_pass https://auth.pingone.com/<env-id>/as/token;
    proxy_set_header Host auth.pingone.com;
    proxy_set_header Authorization "Basic <base64-encoded-publisher-creds>";
    proxy_set_header Content-Type $http_content_type;
    proxy_ssl_server_name on;
}

# Subscriber app
location /api/token/subscriber {
    proxy_pass https://auth.pingone.com/<env-id>/as/token;
    proxy_set_header Host auth.pingone.com;
    proxy_set_header Authorization "Basic <base64-encoded-subscriber-creds>";
    proxy_set_header Content-Type $http_content_type;
    proxy_ssl_server_name on;
}
```

## Step 6: Verify Token Issuance

Test that PingOne issues tokens with the correct scopes:

```bash
# Fetch an admin token
curl -s -X POST "https://auth.pingone.com/<env-id>/as/token" \
  -u "<admin-client-id>:<admin-client-secret>" \
  -d "grant_type=client_credentials&scope=nats:admin" | jq .

# Decode the token (paste at jwt.io or use jq)
# The "scope" claim should contain "nats:admin"
```

Expected token claims:
```json
{
  "iss": "https://auth.pingone.com/<env-id>/as",
  "sub": "<admin-client-id>",
  "client_id": "<admin-client-id>",
  "scope": "nats:admin",
  "exp": 1234567890,
  "iat": 1234567800
}
```

## How It Fits Together

```
PingOne Admin Console
├── Environment: <env-id>
│   ├── Resource: nats-api
│   │   ├── Scope: nats:admin
│   │   ├── Scope: nats:publish
│   │   └── Scope: nats:subscribe
│   │
│   ├── App: nats-admin        → client_credentials → nats:admin
│   ├── App: nats-publisher    → client_credentials → nats:publish
│   └── App: nats-subscriber   → client_credentials → nats:subscribe

Auth Service (Go)
├── OIDC Discovery: https://auth.pingone.com/<env-id>/as/.well-known/openid-configuration
├── JWKS: auto-discovered from provider metadata
└── Scope mapping: nats:admin → {pub: ">", sub: ">"}, etc.

NATS Server
├── TLS: 4222 (clients), WSS: 8443 (browser)
└── auth_callout: delegates ALL auth to auth service
```

## Troubleshooting

### "token validation failed" errors

- Verify the `OIDC_ISSUER_URL` matches your PingOne environment exactly: `https://auth.pingone.com/<env-id>/as`
- Check that the PingOne applications are **enabled** (toggle in the admin console)
- Ensure the auth service can reach `auth.pingone.com` (DNS resolution, firewall rules)

### "no authorized NATS scopes in token"

- Check that each PingOne application has the `nats-api` resource assigned with the correct scope
- Verify the token request includes the `scope` parameter: `scope=nats:admin`
- Decode the token at [jwt.io](https://jwt.io) and confirm the `scope` claim contains the expected NATS scope

### Token proxy returns 401

- Verify the Base64 Authorization header in `nginx.conf` is correct
- The format is `base64(client_id:client_secret)` — no spaces, no newlines
- Confirm the PingOne application's client secret hasn't been rotated
