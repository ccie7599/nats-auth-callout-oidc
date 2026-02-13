# nats-auth-callout-oidc

## Overview

Demonstrates NATS auth-callout delegating client authentication and authorization to PingOne (OIDC). A Go auth-callout service validates bearer tokens via OIDC discovery/JWKS, maps token scopes to NATS pub/sub permissions, and returns signed authorization JWTs to the NATS server.

Three PingOne OIDC applications with distinct scopes demonstrate fine-grained access control: admin (full access), publisher (pub only), and subscriber (sub only).

## Architecture

```
                                     ┌──────────────────┐
                                     │  PingOne          │
                                     │  (OIDC Provider)  │
                                     └────┬─────────┬───┘
                                JWKS/Disc │         │ token
                                          ▼         │
┌──────────────────┐              ┌──────────────┐  │
│  Web Dashboard   │──WSS:8443──▶│  NATS Server  │  │
│  (browser)       │              │  TLS:4222     │  │
│  served HTTPS:443│              │  WSS:8443     │  │
└────────┬─────────┘              └──────┬────────┘  │
         │ fetch /api/token/*            │           │
         ▼                               ▼           │
┌──────────────────┐    $SYS.REQ.USER.AUTH           │
│  Nginx           │              ┌──────────────┐   │
│  HTTPS + proxy   │              │  Auth Service │◀──┘
└──────────────────┘              │  (Go)         │
                                  │  audit.>      │
┌──────────────────┐              └──────────────┘
│  Demo Client     │──TLS:4222──▶ (same NATS)
│  (Go CLI)        │
└──────────────────┘
```

**Flow**: Client obtains PingOne token → connects to NATS (TLS or WSS) with bearer token → NATS delegates to auth-service via `$SYS.REQ.USER.AUTH` → auth-service validates JWT against PingOne JWKS → maps scopes to NATS pub/sub permissions → publishes audit event → returns signed UserClaims JWT → NATS enforces permissions.

## Scope Mapping

| OIDC Scope | Publish Allow | Subscribe Allow | Use Case |
|---|---|---|---|
| `nats:admin` | `>` | `>` | Full access |
| `nats:publish` | `orders.>`, `events.>` | `_INBOX.>` | Service producing events |
| `nats:subscribe` | _(none)_ | `orders.>`, `events.>`, `_INBOX.>` | Consumer/reader service |
| _(no NATS scope)_ | _(denied)_ | _(denied)_ | Connection rejected |

## Quick Start

### Prerequisites

- Docker & Docker Compose
- TLS certs in `./certs/` (Let's Encrypt via `make certs`, or bring your own)
- PingOne trial tenant with 3 OIDC Web Apps (see [PingOne Setup](#pingone-setup))

### Run

```bash
make build    # Build containers
make up       # Start NATS + auth service
make demo     # Run all 5 CLI demo scenarios
make up-all   # Start with dashboard
make down     # Tear down
```

### Demo Scenarios

| # | Scenario | Expected Result |
|---|----------|----------------|
| 1 | Admin (`nats:admin`) | Connect OK, pub/sub to anything |
| 2 | Publisher (`nats:publish`) | Connect OK, pub to orders/events, sub denied |
| 3 | Subscriber (`nats:subscribe`) | Connect OK, sub to orders/events, pub denied |
| 4 | Invalid token | Connection rejected |
| 5 | No token | Connection rejected |

### Interactive Dashboard

Available at `https://<your-domain>/` when running with `make up-all`. Dark-themed single-page app with:
- **Scenario buttons** — click to run each scenario over WSS
- **Message flow panel** — shows pub/sub results with allow/deny indicators
- **Auth audit trail** — real-time auth decisions from the auth-service
- **Live log** — terminal-style connection and test output

### PingOne Setup

1. Create a PingOne trial at https://www.pingidentity.com/en/try-ping.html
2. Create a custom resource `nats-api` with scopes: `nats:admin`, `nats:publish`, `nats:subscribe`
3. Create 3 OIDC Web Apps (each with `client_credentials` grant type):
   - **Admin app** — assign `nats:admin` scope
   - **Publisher app** — assign `nats:publish` scope
   - **Subscriber app** — assign `nats:subscribe` scope
4. Copy `.env.example` to `.env` and fill in credentials for all 3 apps
5. Update `nginx/nginx.conf` Base64 Authorization headers for the token proxy endpoints

### TLS Certificates

```bash
make certs        # Generate via Let's Encrypt + Akamai DNS-01
make certs-check  # Check cert validity
```

Requires `certbot` with the `edgedns` authenticator plugin and `~/.edgerc` credentials.

## Status

- **Tier**: 1 (Demo/POC)
- **Phase**: Complete
- **Live**: https://nats-demo.connected-cloud.io/
- **Scope**: See [SCOPE.md](./SCOPE.md)
