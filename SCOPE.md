# Scope Lock: nats-auth-callout-oidc

## Problem Statement

Demonstrate NATS auth-callout integrating with a generic OIDC identity provider for client authentication and fine-grained authorization (pub/sub permissions mapped from OIDC scopes). Includes a live PingOne scenario to show zero-code-change IdP swappability for enterprise pre-sales.

## Tier Classification

- [x] **Tier 1** — Demo/POC (1-2 days, throwaway OK)
- [ ] **Tier 2** — Reference Architecture (1-2 weeks, reusable)
- [ ] **Tier 3** — Production Candidate (2-6 weeks, hardened)

## Deliverables

1. Go auth-callout service validating OIDC tokens and mapping scopes to NATS pub/sub permissions
2. Docker Compose environment (NATS server, auth service, mock OIDC provider)
3. Go demo client exercising 6 scenarios (admin, publisher, subscriber, invalid token, no token, live PingOne)
4. Documentation: README, architecture, Ping Identity swap guide

## Scale Commitments

| Metric | Designed For | Tested At | Evidence |
|--------|-------------|-----------|----------|
| Concurrent connections | N/A (demo) | — | — |
| Auth callout latency | < 100ms | — | — |
| Token validation | Per-connection | — | — |

## Explicit Non-Goals

1. [ ] **Production hardening** — No rate limiting, circuit breaking, or HA for the auth service
2. [ ] **Multi-region / multi-cluster** — Single Docker Compose stack only
3. [ ] **Token refresh / rotation** — Demo uses short-lived tokens, no refresh flow
4. [ ] **Custom policy engine** — Scope-to-permission mapping is a static Go map, not OPA/Cedar
5. [ ] **TLS / mTLS** — Plain NATS for demo simplicity; production would require TLS

## Exit Criteria

- [ ] `make up && make demo` runs all 6 scenarios end-to-end with expected PASS/FAIL output
- [ ] Auth-service logs show token validation decisions for each scenario
- [ ] Unit tests pass for permission mapping logic
- [ ] PingOne scenario works when trial credentials are configured (skipped gracefully otherwise)
- [ ] README provides < 5-minute quick start
