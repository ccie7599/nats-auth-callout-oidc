# Architecture Decision Records

## ADR-001: Use synadia-io/callout.go Helper Library

**Date:** 2026-02-13
**Status:** Accepted
**Context:** NATS auth-callout requires subscribing to `$SYS.REQ.USER.AUTH`, decoding JWT requests, building signed response JWTs, and handling encryption. We can implement this from scratch using `nats-io/jwt/v2` or use the official Synadia helper library.
**Decision:** Use `github.com/synadia-io/callout.go` which wraps the callout protocol, handles request decoding, response signing, and error wrapping.
**Alternatives Considered:** Raw subscription to `$SYS.REQ.USER.AUTH` with manual JWT handling — more code, more surface area for protocol bugs.
**Consequences:** Additional dependency, but maintained by the NATS team. Reduces auth-callout boilerplate to a single `Authorizer` function.

## ADR-002: NKey Generation via Init Container

**Date:** 2026-02-13
**Status:** Accepted
**Context:** Auth-callout requires a shared NKey pair — the public key goes in NATS server config (`issuer`), the private seed goes to the auth service for signing response JWTs. We need to manage this without committing secrets to source.
**Decision:** Generate NKeys at Docker Compose startup via an init container (`natsio/nats-box`), write to a shared Docker volume. Both NATS server and auth-service read from the volume.
**Alternatives Considered:** Pre-generated keys in `.env` file (simpler but puts secrets in source); Vault-managed keys (overkill for Tier 1 demo).
**Consequences:** Each `docker compose up` gets fresh keys. No secrets in git. Adds startup dependency on init container.

## ADR-003: Token Transport via nats.Token()

**Date:** 2026-02-13
**Status:** Accepted
**Context:** NATS clients need to pass the OIDC bearer token to the server. Multiple fields in CONNECT options could carry it: `auth_token`, `user`/`pass`, or a custom JWT.
**Decision:** Primary: `nats.Token(bearerToken)` which maps to `ConnectOptions.Token` (JSON: `auth_token`). Fallback: also check `ConnectOptions.Password` for clients that use `nats.UserInfo("", bearerToken)`.
**Alternatives Considered:** NATS JWT-based auth (requires nsc tooling, more complex for OIDC integration); custom headers (not supported by NATS protocol).
**Consequences:** Works across all NATS client libraries. Simple for demo clients.

## ADR-004: Multi-Issuer OIDC Verification

**Date:** 2026-02-13
**Status:** Accepted
**Context:** The demo needs to validate tokens from both the local mock OIDC provider and (optionally) a live PingOne tenant simultaneously.
**Decision:** Auth-service accepts comma-separated `OIDC_ISSUER_URL` values, initializes a verifier for each, and tries them in order during token validation.
**Alternatives Considered:** Single issuer with environment swap (simpler but can't demo both simultaneously); audience-based routing (requires consistent audience claims across providers).
**Consequences:** Both mock and PingOne tokens work against the same NATS cluster. Slight validation overhead for multi-issuer try-each approach.
