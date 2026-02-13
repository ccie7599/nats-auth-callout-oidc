# Ping Identity Integration Guide

## PingOne Trial Setup

### 1. Create a Trial Account

1. Go to https://www.pingidentity.com/en/try-ping.html
2. Sign up with your business email
3. You get a 30-day free trial with up to 1,000 active identities and 5 environments

### 2. Create a Worker Application

1. In PingOne admin console, navigate to **Applications** > **Applications**
2. Click **+** to add a new application
3. Select **Worker** as the application type
4. Name it (e.g., `nats-demo-admin`)
5. Click **Save**

### 3. Configure OAuth2 Scopes

1. Navigate to **Authorization** > **Custom Resources**
2. Create a new resource called `nats-api`
3. Add custom scopes:
   - `nats:admin` — Full NATS access
   - `nats:publish` — Publish to orders/events subjects
   - `nats:subscribe` — Subscribe to orders/events subjects
4. Go back to your application, open the **Resources** tab
5. Grant the desired scope(s) to the application

### 4. Collect Credentials

From the application's **Configuration** tab, note:
- **Environment ID** (from the URL or environment settings)
- **Client ID**
- **Client Secret** (click to reveal/copy)

The OIDC issuer URL follows the pattern:
```
https://auth.pingone.com/<environment-id>/as
```

Verify by fetching the discovery document:
```bash
curl https://auth.pingone.com/<environment-id>/as/.well-known/openid-configuration | jq .
```

### 5. Configure the Demo

Copy `.env.example` to `.env` and fill in your values:
```bash
cp .env.example .env
```

```env
PING_ISSUER_URL=https://auth.pingone.com/<environment-id>/as
PING_CLIENT_ID=<your-client-id>
PING_CLIENT_SECRET=<your-client-secret>
PING_SCOPE=nats:admin
```

### 6. Run the Live Demo

```bash
make up         # Start services (auth-service will initialize verifiers for both mock + PingOne)
make demo-ping  # Run only the PingOne scenario
```

## Swapping to PingOne as the Only Provider

To use PingOne exclusively (no mock provider), set `OIDC_ISSUER_URL` in `docker-compose.yml`:

```yaml
auth-service:
  environment:
    OIDC_ISSUER_URL: "https://auth.pingone.com/<environment-id>/as"
```

Remove the `oidc-provider` service from `docker-compose.yml`. No code changes needed.

## PingFederate (Self-Hosted)

The same approach works with PingFederate:

1. Configure an OAuth client with `client_credentials` grant type
2. Set up an Access Token Manager that issues JWT tokens
3. Add custom scope-to-claim mappings for `nats:admin`, `nats:publish`, `nats:subscribe`
4. Set the issuer URL to your PingFederate instance:
   ```
   PING_ISSUER_URL=https://<pingfederate-host>:9031
   ```

## Any OIDC Provider

This demo works with any OIDC-compliant provider. The auth-service uses standard OIDC discovery (`.well-known/openid-configuration`) and JWKS for token validation. To swap providers:

1. Register an application/client with `client_credentials` grant
2. Create custom scopes: `nats:admin`, `nats:publish`, `nats:subscribe`
3. Set `OIDC_ISSUER_URL` to the provider's issuer URL
