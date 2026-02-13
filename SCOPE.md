# Scope Lock: nats-auth-callout-oidc

## Problem Statement

Demonstrate NATS auth-callout integrating with a generic OIDC identity provider for client authentication and fine-grained authorization (pub/sub permissions mapped from OIDC scopes). Includes a live PingOne scenario to show zero-code-change IdP swappability for enterprise pre-sales.

## Tier Classification

- [x] **Tier 1** — Demo/POC (1-2 days, throwaway OK)
- [ ] **Tier 2** — Reference Architecture (1-2 weeks, reusable)
- [ ] **Tier 3** — Production Candidate (2-6 weeks, hardened)

## Deliverables

1. Go auth-callout service validating OIDC tokens and mapping scopes to NATS pub/sub permissions
2. Docker Compose environment (NATS server w/ TLS + WSS, auth service, nginx dashboard)
3. Go demo client exercising 5 scenarios against PingOne (admin, publisher, subscriber, invalid token, no token)
4. Interactive browser dashboard over WSS (dark theme, audit trail, scenario buttons)
5. TLS everywhere: Let's Encrypt certs via Akamai DNS-01 on `nats-demo.connected-cloud.io`
6. Documentation: README, architecture, decisions

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
5. [x] ~~**TLS / mTLS** — Plain NATS for demo simplicity~~ — TLS implemented with Let's Encrypt certs

## Exit Criteria

- [x] `make up && make demo` runs all 5 scenarios end-to-end with expected PASS/FAIL output
- [x] Auth-service logs show token validation decisions for each scenario
- [ ] Unit tests pass for permission mapping logic
- [x] PingOne is the sole identity provider — all scenarios validated against live PingOne tenant
- [x] Interactive dashboard live at https://nats-demo.connected-cloud.io/
- [x] README provides < 5-minute quick start
