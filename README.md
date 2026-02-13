# nats-auth-callout-oidc

## Overview

Demonstrates NATS auth-callout delegating client authentication and authorization to an OIDC identity provider. A Go auth-callout service validates bearer tokens via standard OIDC discovery/JWKS, maps token scopes to NATS pub/sub permissions, and returns signed authorization JWTs to the NATS server.

Designed to be provider-agnostic — ships with a mock OIDC provider for local dev and supports live PingOne/PingFederate by changing one environment variable.

## Architecture

```
┌─────────────┐    OIDC Token     ┌──────────────┐
│  NATS Client │───────────────────│  OIDC Provider│
│  (demo-client)│  (bearer token)  │  (mock/Ping)  │
└──────┬───────┘                   └──────┬────────┘
       │ CONNECT w/ token                 │ JWKS + Discovery
       ▼                                  ▼
┌──────────────┐  $SYS.REQ.USER.AUTH  ┌──────────────┐
│  NATS Server  │────────────────────▶│  Auth Service │
│  (auth_callout)│◀───────────────────│  (Go)         │
└───────────────┘  signed UserClaims  └──────────────┘
```

**Flow**: Client obtains OIDC token → connects to NATS with token as bearer credential → NATS delegates to auth-service via auth-callout → auth-service validates token against OIDC provider → maps scopes to NATS permissions → returns signed UserClaims JWT → NATS enforces permissions.

## Scope Mapping

| OIDC Scope | Publish Allow | Subscribe Allow | Use Case |
|---|---|---|---|
| `nats:admin` | `>` | `>` | Full access |
| `nats:publish` | `orders.>`, `events.>` | `_INBOX.>` | Service producing events |
| `nats:subscribe` | _(none)_ | `orders.>`, `events.>`, `_INBOX.>` | Consumer/reader service |
| _(no NATS scope)_ | _(denied)_ | _(denied)_ | Connection rejected |

## Quick Start

```bash
make build    # Build containers
make up       # Start NATS, auth service, mock OIDC provider
make demo     # Run all 6 demo scenarios
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
| 6 | Live PingOne | Same as admin, against real IdP (requires config) |

### PingOne Live Demo (Optional)

1. Sign up for a free 30-day trial at https://www.pingidentity.com/en/try-ping.html
2. Copy `.env.example` to `.env` and fill in your PingOne credentials
3. Run `make demo-ping`

See [docs/ping-identity-guide.md](docs/ping-identity-guide.md) for detailed PingOne setup instructions.

## Status

- **Tier**: 1 (Demo/POC)
- **Phase**: Initial Setup
- **Scope**: See [SCOPE.md](./SCOPE.md)
